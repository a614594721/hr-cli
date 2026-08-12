package auth

import (
	"context"

	"hr-cli/internal/errs"
	"hr-cli/internal/runtime"
)

// Operator is the identity returned by hr-gateway /auth/me. The CLI never
// fabricates an operator: every field comes from the JWT validated by the
// gateway. Source is "dingtalk_oauth" for any session backed by the broker.
type Operator struct {
	EID    string `json:"eid,omitempty"`
	URID   string `json:"urid,omitempty"`
	Badge  string `json:"badge,omitempty"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Source string `json:"source"`
}

// LoginRequest captures the flag inputs for `hr auth +login`. Legacy direct-DB
// identifier flags (--eid/--badge/...) remain accepted for CLI compatibility
// but are ignored by both supported HTTP-based providers.
type LoginRequest struct {
	EID            int
	Badge          string
	Email          string
	Phone          string
	Name           string
	DingUserID     string
	Role           string
	DingTalk       bool
	AuthBaseURL    string
	NoBrowser      bool
	NoWait         bool
	LoginID        string
	LoginSecret    string
	Provider       string
	ResumeID       string
	TimeoutSeconds int
}

func Me() (Operator, *errs.Error) {
	session, hasSession := runtime.LoadSession()
	if !hasSession {
		return Operator{}, errs.Authentication("not_logged_in", "no active authentication session; run hr auth +login")
	}
	refreshed, err := refreshSessionIfNeeded(context.Background(), session, false)
	if err != nil {
		return Operator{}, err
	}
	stored, err := loadStoredToken(refreshed)
	if err != nil {
		return Operator{}, err
	}
	if stored == nil {
		return Operator{}, errs.Authentication("missing_token", "authentication token is missing; "+loginHint(session))
	}
	operator, _, err := remoteMe(refreshed, stored)
	if err != nil {
		refreshed, err = refreshSessionIfNeeded(context.Background(), refreshed, true)
		if err != nil {
			return Operator{}, err
		}
		stored, err = loadStoredToken(refreshed)
		if err != nil {
			return Operator{}, err
		}
		if stored == nil {
			return Operator{}, errs.Authentication("missing_token", "authentication token is missing; "+loginHint(session))
		}
		operator, _, err = remoteMe(refreshed, stored)
		if err != nil {
			return Operator{}, err
		}
	}
	refreshed.EID = operator.EID
	refreshed.URID = operator.URID
	refreshed.Badge = operator.Badge
	refreshed.Name = operator.Name
	refreshed.Role = operator.Role
	if operator.Source != "" {
		refreshed.Source = operator.Source
	}
	if saveErr := runtime.SaveSession(refreshed); saveErr != nil {
		return Operator{}, saveErr
	}
	if operator.Source == "" {
		operator.Source = refreshed.Source
		if operator.Source == "" {
			operator.Source = sessionProvider(refreshed)
		}
	}
	return operator, nil
}

func Status(verify bool) (map[string]any, *errs.Error) {
	session, hasSession := runtime.LoadSession()
	if !hasSession {
		return map[string]any{"status": "no_session", "verified": false}, nil
	}
	if verify {
		operator, err := Me()
		if err != nil {
			return nil, err
		}
		session, hasSession = runtime.LoadSession()
		return statusData(operator, session, hasSession, true)
	}
	operator := sessionToOperator(session)
	return statusData(operator, session, true, false)
}

func statusData(operator Operator, session runtime.Session, hasSession bool, verified bool) (map[string]any, *errs.Error) {
	provider := sessionProvider(session)
	mode := provider
	if provider == ProviderLegacyGateway {
		mode = "dingtalk_oauth"
	}
	status := "active"
	data := map[string]any{"status": status, "mode": mode, "provider": provider, "operator": operator, "verified": verified}
	if hasSession {
		stored, err := loadStoredToken(session)
		if err != nil {
			return nil, err
		}
		tokenState := tokenStatus(stored)
		if tokenState == "expired" {
			data["status"] = "expired"
		}
		if tokenState == "missing" {
			data["status"] = "missing_token"
		}
		if stored != nil {
			data["access_expires_at"] = stored.AccessTokenExpiresAt
			data["refresh_expires_at"] = stored.RefreshTokenExpiresAt
			data["granted_at"] = stored.GrantedAt
		} else {
			data["access_expires_at"] = session.AccessTokenExpiresAt
			data["refresh_expires_at"] = session.RefreshTokenExpiresAt
		}
		data["token_status"] = tokenState
		data["auth_base_url"] = session.AuthBaseURL
		data["provider"] = sessionProvider(session)
		if session.Issuer != "" {
			data["issuer"] = session.Issuer
			data["client_id"] = session.ClientID
			data["audience"] = session.Audience
			data["scope"] = session.Scope
		}
	}
	return data, nil
}

// Login is the provider-neutral entry point for `hr auth +login`. Legacy
// gateway login remains the default until Hydra is explicitly selected.
func Login(ctx context.Context, req LoginRequest) (map[string]any, *errs.Error) {
	provider, err := resolveLoginProvider(req)
	if err != nil {
		return nil, err
	}
	if provider == ProviderHydra {
		return LoginHydra(ctx, req)
	}
	return LoginDingTalk(req)
}

func Logout() (map[string]any, *errs.Error) {
	session, hasSession := runtime.LoadSession()
	remoteRevoked := false
	provider := ProviderLegacyGateway
	mode := "dingtalk_oauth"
	if hasSession {
		provider = sessionProvider(session)
		if provider == ProviderHydra {
			mode = ProviderHydra
			remoteRevoked = revokeHydraSession(context.Background(), session)
		} else if session.AuthBaseURL != "" {
			remoteRevoked = revokeRemoteSession(session)
		}
		if err := removeStoredToken(session); err != nil {
			return nil, err
		}
	}
	removed, err := runtime.ClearSession()
	if err != nil {
		return nil, err
	}
	status := "no_session"
	if removed {
		status = "cleared"
	}
	return map[string]any{"status": status, "mode": mode, "provider": provider, "remote_revoked": remoteRevoked}, nil
}

// AccessToken returns a usable access token from the active provider,
// refreshing it in-place if it is within the refresh-ahead window. It returns
// ("", nil) when no authentication session is configured.
//
// force=true bypasses the freshness check and always rotates the token —
// used by the gateway client after a 401/token_expired response.
func AccessToken(force bool) (string, *errs.Error) {
	session, ok := runtime.LoadSession()
	if !ok {
		return "", nil
	}
	refreshed, err := refreshSessionIfNeeded(context.Background(), session, force)
	if err != nil {
		return "", err
	}
	stored, err := loadStoredToken(refreshed)
	if err != nil {
		return "", err
	}
	if stored == nil {
		return "", errs.Authentication("missing_token", "authentication token is missing; "+loginHint(session))
	}
	return stored.AccessToken, nil
}

func sessionToOperator(session runtime.Session) Operator {
	return Operator{EID: session.EID, URID: session.URID, Badge: session.Badge, Name: session.Name, Role: session.Role, Source: session.Source}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
