package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"hr-cli/internal/errs"
	"hr-cli/internal/keychain"
	appruntime "hr-cli/internal/runtime"
)

const (
	defaultHydraClientID = "hr-cli"
	defaultHydraAudience = "hr-gateway"
	defaultHydraScope    = "openid offline_access"
	deviceGrantType      = "urn:ietf:params:oauth:grant-type:device_code"
)

type hydraEndpoints struct {
	Issuer                      string
	DeviceAuthorizationEndpoint string
	TokenEndpoint               string
	RevocationEndpoint          string
}

type hydraDiscovery struct {
	Issuer                      string `json:"issuer"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	RevocationEndpoint          string `json:"revocation_endpoint"`
}

type deviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type hydraTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type pendingDeviceAuthorization struct {
	ResumeID                    string `json:"resumeId"`
	AuthBaseURL                 string `json:"authBaseUrl"`
	Issuer                      string `json:"issuer"`
	DeviceAuthorizationEndpoint string `json:"deviceAuthorizationEndpoint"`
	TokenEndpoint               string `json:"tokenEndpoint"`
	RevocationEndpoint          string `json:"revocationEndpoint"`
	ClientID                    string `json:"clientId"`
	Audience                    string `json:"audience"`
	Scope                       string `json:"scope"`
	DeviceCode                  string `json:"deviceCode"`
	UserCode                    string `json:"userCode"`
	VerificationURI             string `json:"verificationUri"`
	VerificationURIComplete     string `json:"verificationUriComplete"`
	ExpiresAt                   string `json:"expiresAt"`
	Interval                    int    `json:"interval"`
}

type hydraClient struct {
	httpClient *http.Client
	now        func() time.Time
	sleep      func(context.Context, time.Duration) error
}

func newHydraClient() *hydraClient {
	return &hydraClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now: time.Now,
		sleep: func(ctx context.Context, duration time.Duration) error {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

func LoginHydra(ctx context.Context, req LoginRequest) (map[string]any, *errs.Error) {
	profile, _ := appruntime.ActiveProfile()
	client := newHydraClient()
	var pending pendingDeviceAuthorization
	var err *errs.Error
	if req.ResumeID != "" {
		pending, err = loadPendingDeviceAuthorization(req.ResumeID)
		if err != nil {
			return nil, err
		}
	} else {
		config, configErr := resolveHydraConfig(ctx, client, profile, req.AuthBaseURL)
		if configErr != nil {
			return nil, configErr
		}
		device, deviceErr := client.requestDeviceAuthorization(ctx, config)
		if deviceErr != nil {
			return nil, deviceErr
		}
		pending, err = pendingFromDevice(config, device)
		if err != nil {
			return nil, err
		}
		if saveErr := savePendingDeviceAuthorization(pending); saveErr != nil {
			return nil, saveErr
		}
	}

	verificationURL := firstNonEmpty(pending.VerificationURIComplete, pending.VerificationURI)
	if req.NoWait && req.ResumeID == "" {
		return pendingDeviceData(pending), nil
	}
	if req.ResumeID == "" {
		if !req.NoBrowser && verificationURL != "" {
			if openErr := openBrowser(verificationURL); openErr != nil {
				fmt.Fprintf(os.Stderr, "failed to open browser automatically: %v\n", openErr)
			}
		}
		fmt.Fprintf(os.Stderr, "Verification URL: %s\n", pending.VerificationURI)
		fmt.Fprintf(os.Stderr, "User code: %s\n", pending.UserCode)
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	token, pollErr := client.pollDeviceToken(ctx, pending, timeout)
	if pollErr != nil {
		if pollErr.Subtype == "device_code_expired" || pollErr.Subtype == "access_denied" {
			_ = removePendingDeviceAuthorization(pending.ResumeID)
		}
		return nil, pollErr
	}
	_ = removePendingDeviceAuthorization(pending.ResumeID)

	session, stored := hydraSessionAndToken(pending, token)
	if saveErr := storeStoredToken(session, &stored); saveErr != nil {
		return nil, saveErr
	}
	if saveErr := appruntime.SaveSession(session); saveErr != nil {
		return nil, saveErr
	}
	return map[string]any{
		"status":            "active",
		"mode":              ProviderHydra,
		"provider":          ProviderHydra,
		"issuer":            session.Issuer,
		"client_id":         session.ClientID,
		"audience":          session.Audience,
		"scope":             session.Scope,
		"access_expires_at": session.AccessTokenExpiresAt,
	}, nil
}

func resolveHydraConfig(ctx context.Context, client *hydraClient, profile appruntime.Profile, authBaseURLOverride string) (pendingDeviceAuthorization, *errs.Error) {
	issuer := strings.TrimRight(strings.TrimSpace(firstNonEmpty(os.Getenv("HR_HYDRA_ISSUER"), profile.HydraIssuer)), "/")
	if issuer != "" && !isSecureOAuthURL(issuer) {
		return pendingDeviceAuthorization{}, errs.Config("hydra_issuer_insecure", "Hydra issuer must use HTTPS except for localhost development")
	}
	endpoints := hydraEndpoints{
		Issuer:                      issuer,
		DeviceAuthorizationEndpoint: firstNonEmpty(os.Getenv("HR_HYDRA_DEVICE_AUTHORIZATION_URL"), profile.HydraDeviceAuthorizationURL),
		TokenEndpoint:               firstNonEmpty(os.Getenv("HR_HYDRA_TOKEN_URL"), profile.HydraTokenURL),
		RevocationEndpoint:          firstNonEmpty(os.Getenv("HR_HYDRA_REVOCATION_URL"), profile.HydraRevocationURL),
	}
	if endpoints.DeviceAuthorizationEndpoint == "" || endpoints.TokenEndpoint == "" || endpoints.RevocationEndpoint == "" {
		if issuer == "" {
			return pendingDeviceAuthorization{}, errs.Config("hydra_endpoints_missing", "Hydra mode requires hydra_issuer or explicit device, token, and revoke endpoints")
		}
		discovered, err := client.discover(ctx, issuer)
		if err != nil {
			return pendingDeviceAuthorization{}, err
		}
		endpoints.Issuer = firstNonEmpty(discovered.Issuer, endpoints.Issuer)
		endpoints.DeviceAuthorizationEndpoint = firstNonEmpty(endpoints.DeviceAuthorizationEndpoint, discovered.DeviceAuthorizationEndpoint)
		endpoints.TokenEndpoint = firstNonEmpty(endpoints.TokenEndpoint, discovered.TokenEndpoint)
		endpoints.RevocationEndpoint = firstNonEmpty(endpoints.RevocationEndpoint, discovered.RevocationEndpoint)
	}
	if endpoints.Issuer == "" {
		endpoints.Issuer = endpointAuthority(endpoints.TokenEndpoint)
	}
	for name, endpoint := range map[string]string{
		"device authorization": endpoints.DeviceAuthorizationEndpoint,
		"token":                endpoints.TokenEndpoint,
		"revocation":           endpoints.RevocationEndpoint,
	} {
		if !isSecureOAuthURL(endpoint) {
			return pendingDeviceAuthorization{}, errs.Config("hydra_endpoint_invalid", fmt.Sprintf("Hydra %s endpoint must use HTTPS except for localhost development", name))
		}
	}
	authBaseURL, err := authBaseURL(authBaseURLOverride)
	if err != nil {
		return pendingDeviceAuthorization{}, err
	}
	clientID := firstNonEmpty(os.Getenv("HR_HYDRA_CLIENT_ID"), profile.HydraClientID, defaultHydraClientID)
	if clientID != defaultHydraClientID {
		return pendingDeviceAuthorization{}, errs.Config("hydra_client_id_invalid", "hr-cli Hydra client_id must be hr-cli")
	}
	audience := firstNonEmpty(os.Getenv("HR_HYDRA_AUDIENCE"), profile.HydraAudience, defaultHydraAudience)
	if audience != defaultHydraAudience {
		return pendingDeviceAuthorization{}, errs.Config("hydra_audience_invalid", "hr-cli Hydra audience must be hr-gateway")
	}
	return pendingDeviceAuthorization{
		AuthBaseURL:                 authBaseURL,
		Issuer:                      endpoints.Issuer,
		DeviceAuthorizationEndpoint: endpoints.DeviceAuthorizationEndpoint,
		TokenEndpoint:               endpoints.TokenEndpoint,
		RevocationEndpoint:          endpoints.RevocationEndpoint,
		ClientID:                    clientID,
		Audience:                    audience,
		Scope:                       normalizeScopes(firstNonEmpty(os.Getenv("HR_HYDRA_SCOPE"), profile.HydraScope, defaultHydraScope)),
	}, nil
}

func (client *hydraClient) discover(ctx context.Context, issuer string) (hydraDiscovery, *errs.Error) {
	var discovery hydraDiscovery
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return discovery, errs.Config("hydra_discovery_request_failed", err.Error())
	}
	resp, doErr := client.httpClient.Do(req)
	if doErr != nil {
		return discovery, errs.Network("hydra_discovery_failed", doErr.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return discovery, errs.Network("hydra_discovery_failed", fmt.Sprintf("Hydra discovery returned HTTP %d", resp.StatusCode))
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return discovery, errs.Network("hydra_discovery_invalid", err.Error())
	}
	if discovery.Issuer != "" && strings.TrimRight(discovery.Issuer, "/") != issuer {
		return discovery, errs.Config("hydra_issuer_mismatch", "Hydra discovery issuer does not match the configured issuer")
	}
	return discovery, nil
}

func (client *hydraClient) requestDeviceAuthorization(ctx context.Context, config pendingDeviceAuthorization) (deviceAuthorization, *errs.Error) {
	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("scope", config.Scope)
	if config.Audience != "" {
		form.Set("audience", config.Audience)
	}
	var device deviceAuthorization
	if err := client.postForm(ctx, config.DeviceAuthorizationEndpoint, form, &device); err != nil {
		return device, err
	}
	if device.DeviceCode == "" || device.UserCode == "" || device.VerificationURI == "" {
		return device, errs.Network("hydra_device_response_invalid", "Hydra returned an incomplete device authorization response")
	}
	if err := validateVerificationURLs(device); err != nil {
		return device, err
	}
	if device.ExpiresIn <= 0 {
		device.ExpiresIn = 600
	}
	if device.Interval <= 0 {
		device.Interval = 5
	}
	return device, nil
}

func (client *hydraClient) pollDeviceToken(ctx context.Context, pending pendingDeviceAuthorization, timeout time.Duration) (hydraTokenResponse, *errs.Error) {
	var token hydraTokenResponse
	expiresAt, ok := parseTokenTime(pending.ExpiresAt)
	if !ok {
		return token, errs.Authentication("device_code_expired", "pending device authorization has an invalid expiry; start login again")
	}
	deadline := expiresAt
	if timeout > 0 && client.now().Add(timeout).Before(deadline) {
		deadline = client.now().Add(timeout)
	}
	interval := time.Duration(pending.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	for client.now().Before(deadline) {
		remaining := deadline.Sub(client.now())
		if interval >= remaining {
			break
		}
		if err := client.sleep(ctx, interval); err != nil {
			return token, errs.Authentication("login_cancelled", "device authorization polling was cancelled")
		}
		form := url.Values{}
		form.Set("grant_type", deviceGrantType)
		form.Set("device_code", pending.DeviceCode)
		form.Set("client_id", pending.ClientID)
		response, oauthErr, err := client.postOAuthForm(ctx, pending.TokenEndpoint, form)
		if err != nil {
			return token, err
		}
		if oauthErr == nil {
			if err := json.Unmarshal(response, &token); err != nil || token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn <= 0 || !strings.EqualFold(token.TokenType, "Bearer") {
				return token, errs.Network("hydra_token_response_invalid", "Hydra returned an invalid token response")
			}
			return token, nil
		}
		switch oauthErr.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return token, oauthAuthenticationError("access_denied", oauthErr, "device authorization was denied")
		case "expired_token", "invalid_grant":
			return token, oauthAuthenticationError("device_code_expired", oauthErr, "device code expired; start login again")
		case "temporarily_unavailable", "server_error":
			return token, oauthNetworkError("hydra_token_unavailable", oauthErr, "Hydra token service is temporarily unavailable")
		case "invalid_client", "unauthorized_client":
			return token, oauthConfigError("hydra_client_rejected", oauthErr, "Hydra rejected the hr-cli public client configuration")
		default:
			return token, oauthAuthenticationError("hydra_token_error", oauthErr, "Hydra rejected the device token request")
		}
	}
	if !client.now().Before(expiresAt) {
		return token, errs.Authentication("device_code_expired", "device code expired; start login again")
	}
	return token, errs.Network("login_timeout", "device authorization is still pending; resume with the same resume_id")
}

func refreshHydraSessionIfNeeded(ctx context.Context, session appruntime.Session, force bool) (appruntime.Session, *errs.Error) {
	stored, err := loadStoredToken(session)
	if err != nil {
		return session, err
	}
	status := tokenStatus(stored)
	if status == "missing" {
		return session, errs.Authentication("missing_token", "Hydra token is missing; "+loginHint(session))
	}
	if !force && status == "valid" {
		return sessionFromStoredToken(session, stored), nil
	}
	if stored.RefreshToken == "" || status == "expired" {
		_ = removeStoredToken(session)
		return session, errs.Authentication("refresh_token_expired", "Hydra refresh token is unavailable or expired; "+loginHint(session))
	}
	lock, lockErr := acquireTokenLock(session, 30*time.Second)
	if lockErr != nil {
		return session, lockErr
	}
	defer lock.release()
	stored, err = loadStoredToken(session)
	if err != nil {
		return session, err
	}
	status = tokenStatus(stored)
	if status == "missing" {
		return session, errs.Authentication("missing_token", "Hydra token is missing; "+loginHint(session))
	}
	if !force && status == "valid" {
		return sessionFromStoredToken(session, stored), nil
	}
	if stored.RefreshToken == "" || status == "expired" {
		_ = removeStoredToken(session)
		return session, errs.Authentication("refresh_token_expired", "Hydra refresh token is unavailable or expired; "+loginHint(session))
	}
	client := newHydraClient()
	token, oauthErr, requestErr := client.refreshToken(ctx, session.TokenURL, session.ClientID, stored.RefreshToken)
	if requestErr != nil {
		return session, requestErr
	}
	if oauthErr != nil {
		if oauthErr.Error == "invalid_grant" || oauthErr.Error == "invalid_token" {
			_ = removeStoredToken(session)
			return session, oauthAuthenticationError("refresh_invalid", oauthErr, "Hydra rejected the refresh token; login again")
		}
		if oauthErr.Error == "temporarily_unavailable" || oauthErr.Error == "server_error" {
			return session, oauthNetworkError("hydra_refresh_unavailable", oauthErr, "Hydra refresh service is temporarily unavailable")
		}
		if oauthErr.Error == "invalid_client" || oauthErr.Error == "unauthorized_client" {
			return session, oauthConfigError("hydra_client_rejected", oauthErr, "Hydra rejected the hr-cli public client configuration")
		}
		return session, oauthAuthenticationError("refresh_invalid", oauthErr, "Hydra rejected the refresh request")
	}
	if token.RefreshToken == "" {
		token.RefreshToken = stored.RefreshToken
	}
	updatedSession, updated := hydraSessionAndToken(pendingDeviceAuthorization{
		AuthBaseURL:        session.AuthBaseURL,
		Issuer:             session.Issuer,
		TokenEndpoint:      session.TokenURL,
		RevocationEndpoint: session.RevocationURL,
		ClientID:           session.ClientID,
		Audience:           session.Audience,
		Scope:              firstNonEmpty(token.Scope, session.Scope),
	}, token)
	updatedSession.EID = session.EID
	updatedSession.URID = session.URID
	updatedSession.Badge = session.Badge
	updatedSession.Name = session.Name
	updatedSession.Role = session.Role
	updatedSession.CreatedAt = session.CreatedAt
	if saveErr := storeStoredToken(updatedSession, &updated); saveErr != nil {
		return session, saveErr
	}
	if saveErr := appruntime.SaveSession(updatedSession); saveErr != nil {
		return session, saveErr
	}
	return updatedSession, nil
}

func revokeHydraSession(ctx context.Context, session appruntime.Session) bool {
	stored, err := loadStoredToken(session)
	if err != nil || stored == nil || session.RevocationURL == "" {
		return false
	}
	client := newHydraClient()
	attempted := false
	revoked := true
	if stored.RefreshToken != "" {
		attempted = true
		revoked = client.revokeToken(ctx, session.RevocationURL, session.ClientID, stored.RefreshToken, "refresh_token") == nil && revoked
	}
	if stored.AccessToken != "" {
		attempted = true
		revoked = client.revokeToken(ctx, session.RevocationURL, session.ClientID, stored.AccessToken, "access_token") == nil && revoked
	}
	return attempted && revoked
}

func (client *hydraClient) refreshToken(ctx context.Context, endpoint, clientID, refreshToken string) (hydraTokenResponse, *oauthErrorResponse, *errs.Error) {
	var token hydraTokenResponse
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	response, oauthErr, err := client.postOAuthForm(ctx, endpoint, form)
	if err != nil || oauthErr != nil {
		return token, oauthErr, err
	}
	if err := json.Unmarshal(response, &token); err != nil || token.AccessToken == "" || token.ExpiresIn <= 0 || !strings.EqualFold(token.TokenType, "Bearer") {
		return token, nil, errs.Network("hydra_token_response_invalid", "Hydra returned an invalid refresh response")
	}
	return token, nil, nil
}

func (client *hydraClient) revokeToken(ctx context.Context, endpoint, clientID, token, hint string) *errs.Error {
	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", hint)
	form.Set("client_id", clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errs.Network("hydra_revoke_request_failed", err.Error())
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, doErr := client.httpClient.Do(req)
	if doErr != nil {
		return errs.Network("hydra_revoke_failed", doErr.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errs.Network("hydra_revoke_failed", fmt.Sprintf("Hydra revocation returned HTTP %d", resp.StatusCode))
	}
	return nil
}

func (client *hydraClient) postForm(ctx context.Context, endpoint string, form url.Values, out any) *errs.Error {
	response, oauthErr, err := client.postOAuthForm(ctx, endpoint, form)
	if err != nil {
		return err
	}
	if oauthErr != nil {
		switch oauthErr.Error {
		case "temporarily_unavailable", "server_error":
			return oauthNetworkError("hydra_oauth_unavailable", oauthErr, "Hydra OAuth service is temporarily unavailable")
		case "invalid_client", "unauthorized_client":
			return oauthConfigError("hydra_client_rejected", oauthErr, "Hydra rejected the hr-cli public client configuration")
		default:
			return oauthAuthenticationError("hydra_oauth_error", oauthErr, "Hydra rejected the OAuth request")
		}
	}
	if err := json.Unmarshal(response, out); err != nil {
		return errs.Network("hydra_response_invalid", err.Error())
	}
	return nil
}

func (client *hydraClient) postOAuthForm(ctx context.Context, endpoint string, form url.Values) ([]byte, *oauthErrorResponse, *errs.Error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, errs.Network("hydra_request_failed", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, doErr := client.httpClient.Do(req)
	if doErr != nil {
		return nil, nil, errs.Network("hydra_request_failed", doErr.Error())
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, nil, errs.Network("hydra_response_read_failed", readErr.Error())
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil, nil
	}
	var oauthErr oauthErrorResponse
	if err := json.Unmarshal(body, &oauthErr); err != nil || oauthErr.Error == "" {
		return nil, nil, errs.Network("hydra_http_error", fmt.Sprintf("Hydra returned HTTP %d", resp.StatusCode))
	}
	return body, &oauthErr, nil
}

func pendingFromDevice(config pendingDeviceAuthorization, device deviceAuthorization) (pendingDeviceAuthorization, *errs.Error) {
	resumeID, err := randomResumeID()
	if err != nil {
		return pendingDeviceAuthorization{}, errs.New("internal", "device_authorization_id_failed", err.Error(), 5)
	}
	config.ResumeID = resumeID
	config.DeviceCode = device.DeviceCode
	config.UserCode = device.UserCode
	config.VerificationURI = device.VerificationURI
	config.VerificationURIComplete = device.VerificationURIComplete
	config.ExpiresAt = time.Now().Add(time.Duration(device.ExpiresIn) * time.Second).Format(time.RFC3339)
	config.Interval = device.Interval
	return config, nil
}

func validateVerificationURLs(device deviceAuthorization) *errs.Error {
	if !isSecureOAuthURL(device.VerificationURI) {
		return errs.Network("hydra_device_response_invalid", "Hydra returned an insecure verification_uri")
	}
	if device.VerificationURIComplete != "" && !isSecureOAuthURL(device.VerificationURIComplete) {
		return errs.Network("hydra_device_response_invalid", "Hydra returned an insecure verification_uri_complete")
	}
	if device.VerificationURIComplete != "" && endpointAuthority(device.VerificationURIComplete) != endpointAuthority(device.VerificationURI) {
		return errs.Network("hydra_device_response_invalid", "Hydra verification_uri_complete must use the verification_uri origin")
	}
	return nil
}

func hydraSessionAndToken(pending pendingDeviceAuthorization, token hydraTokenResponse) (appruntime.Session, StoredToken) {
	now := time.Now()
	scope := firstNonEmpty(token.Scope, pending.Scope)
	expiresAt := now.Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339)
	session := appruntime.Session{
		Provider:             ProviderHydra,
		AuthBaseURL:          pending.AuthBaseURL,
		Issuer:               pending.Issuer,
		ClientID:             pending.ClientID,
		Audience:             pending.Audience,
		Scope:                scope,
		TokenURL:             pending.TokenEndpoint,
		RevocationURL:        pending.RevocationEndpoint,
		AccessTokenExpiresAt: expiresAt,
	}
	stored := StoredToken{
		Provider:             ProviderHydra,
		AuthBaseURL:          pending.AuthBaseURL,
		Issuer:               pending.Issuer,
		ClientID:             pending.ClientID,
		TokenType:            token.TokenType,
		AccessToken:          token.AccessToken,
		AccessTokenExpiresAt: expiresAt,
		RefreshToken:         token.RefreshToken,
		Scope:                scope,
		GrantedAt:            now.Format(time.RFC3339),
	}
	return session, stored
}

func pendingDeviceData(pending pendingDeviceAuthorization) map[string]any {
	return map[string]any{
		"status":                    "pending",
		"mode":                      ProviderHydra,
		"resume_id":                 pending.ResumeID,
		"verification_uri":          pending.VerificationURI,
		"verification_uri_complete": pending.VerificationURIComplete,
		"user_code":                 pending.UserCode,
		"expires_at":                pending.ExpiresAt,
		"poll_interval_seconds":     pending.Interval,
		"next":                      "open verification_uri, approve access, then run hr auth +login --provider hydra --resume-id <resume_id>",
	}
}

func savePendingDeviceAuthorization(pending pendingDeviceAuthorization) *errs.Error {
	data, err := json.Marshal(pending)
	if err != nil {
		return errs.New("internal", "device_authorization_encode_failed", err.Error(), 5)
	}
	if err := keychain.Set(keychain.ServiceName, pendingDeviceAccount(pending.ResumeID), string(data)); err != nil {
		return errs.Config("device_authorization_store_failed", err.Error())
	}
	return nil
}

func loadPendingDeviceAuthorization(resumeID string) (pendingDeviceAuthorization, *errs.Error) {
	var pending pendingDeviceAuthorization
	value, err := keychain.Get(keychain.ServiceName, pendingDeviceAccount(resumeID))
	if err != nil {
		return pending, errs.Config("device_authorization_load_failed", err.Error())
	}
	if value == "" {
		return pending, errs.Authentication("device_authorization_not_found", "pending device authorization was not found; start login again")
	}
	if err := json.Unmarshal([]byte(value), &pending); err != nil {
		return pending, errs.Config("device_authorization_parse_failed", err.Error())
	}
	if expiresAt, ok := parseTokenTime(pending.ExpiresAt); !ok || time.Now().After(expiresAt) {
		_ = removePendingDeviceAuthorization(resumeID)
		return pending, errs.Authentication("device_code_expired", "device code expired; start login again")
	}
	return pending, nil
}

func removePendingDeviceAuthorization(resumeID string) error {
	return keychain.Remove(keychain.ServiceName, pendingDeviceAccount(resumeID))
}

func pendingDeviceAccount(resumeID string) string {
	return "hydra-pending:" + normalizeAccountPart(resumeID)
}

func randomResumeID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func normalizeScopes(scope string) string {
	seen := map[string]bool{}
	items := make([]string, 0)
	for _, item := range strings.Fields(scope) {
		if !seen[item] {
			seen[item] = true
			items = append(items, item)
		}
	}
	if !seen["offline_access"] {
		items = append(items, "offline_access")
	}
	return strings.Join(items, " ")
}

func endpointAuthority(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func isSecureOAuthURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func oauthAuthenticationError(subtype string, oauthErr *oauthErrorResponse, fallback string) *errs.Error {
	return errs.Authentication(subtype, oauthErrorMessage(oauthErr, fallback))
}

func oauthNetworkError(subtype string, oauthErr *oauthErrorResponse, fallback string) *errs.Error {
	return errs.Network(subtype, oauthErrorMessage(oauthErr, fallback))
}

func oauthConfigError(subtype string, oauthErr *oauthErrorResponse, fallback string) *errs.Error {
	return errs.Config(subtype, oauthErrorMessage(oauthErr, fallback))
}

func oauthErrorMessage(oauthErr *oauthErrorResponse, fallback string) string {
	if oauthErr != nil && oauthErr.ErrorDescription != "" {
		return oauthErr.ErrorDescription
	}
	return fallback
}
