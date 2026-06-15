package auth

import (
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

// LoginRequest captures the flag inputs for `hr auth +login`. After the
// gateway cutover the only supported login flow is DingTalk OAuth (DingTalk
// flag implied), so the legacy direct-DB identifier flags (--eid/--badge/...)
// are accepted for backward CLI compatibility but ignored.
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
	TimeoutSeconds int
}

func Me() (Operator, *errs.Error) {
	session, hasSession := runtime.LoadSession()
	if !hasSession || session.Source != "dingtalk_oauth" {
		return Operator{}, errs.Authentication("not_logged_in", "no active DingTalk session; run hr auth +login --dingtalk")
	}
	refreshed, err := refreshSessionIfNeeded(session, false)
	if err != nil {
		return Operator{}, err
	}
	stored, err := loadStoredToken(refreshed)
	if err != nil {
		return Operator{}, err
	}
	if stored == nil {
		return Operator{}, errs.Authentication("missing_token", "DingTalk token is missing; run hr auth +login --dingtalk")
	}
	operator, _, err := remoteMe(refreshed, stored)
	if err != nil {
		refreshed, err = refreshSessionIfNeeded(refreshed, true)
		if err != nil {
			return Operator{}, err
		}
		stored, err = loadStoredToken(refreshed)
		if err != nil {
			return Operator{}, err
		}
		if stored == nil {
			return Operator{}, errs.Authentication("missing_token", "DingTalk token is missing; run hr auth +login --dingtalk")
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
	refreshed.Source = "dingtalk_oauth"
	if saveErr := runtime.SaveSession(refreshed); saveErr != nil {
		return Operator{}, saveErr
	}
	if operator.Source == "" {
		operator.Source = "dingtalk_oauth"
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
	mode := operator.Source
	if mode == "" {
		mode = "dingtalk_oauth"
	}
	status := "active"
	data := map[string]any{"status": status, "mode": mode, "operator": operator, "verified": verified}
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
	}
	return data, nil
}

// Login is the public entry point for `hr auth +login`. The gateway-only CLI
// supports a single login mechanism: DingTalk OAuth via the broker. We accept
// the legacy --dingtalk flag for explicitness but treat it as the default.
func Login(req LoginRequest) (map[string]any, *errs.Error) {
	return LoginDingTalk(req)
}

func Logout() (map[string]any, *errs.Error) {
	session, hasSession := runtime.LoadSession()
	remoteRevoked := false
	if hasSession && session.Source == "dingtalk_oauth" && session.AuthBaseURL != "" {
		remoteRevoked = revokeRemoteSession(session)
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
	return map[string]any{"status": status, "mode": "dingtalk_oauth", "remote_revoked": remoteRevoked}, nil
}

// AccessToken returns a usable DingTalk-broker access token, refreshing it
// in-place if it is within the refresh-ahead window. Returns ("", nil) when
// no DingTalk session is configured.
//
// force=true bypasses the freshness check and always rotates the token —
// used by the gateway client after a 401/token_expired response.
func AccessToken(force bool) (string, *errs.Error) {
	session, ok := runtime.LoadSession()
	if !ok || session.Source != "dingtalk_oauth" {
		return "", nil
	}
	refreshed, err := refreshSessionIfNeeded(session, force)
	if err != nil {
		return "", err
	}
	stored, err := loadStoredToken(refreshed)
	if err != nil {
		return "", err
	}
	if stored == nil {
		return "", errs.Authentication("missing_token", "DingTalk token is missing; run hr auth +login --dingtalk")
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
