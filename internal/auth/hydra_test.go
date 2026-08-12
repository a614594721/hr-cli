package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"hr-cli/internal/errs"
	appruntime "hr-cli/internal/runtime"
)

func TestRequestDeviceAuthorizationUsesPublicClientShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/device/auth" {
			t.Fatalf("path = %q, want /oauth2/device/auth", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header = %q, want empty", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		assertFormValue(t, r.Form, "client_id", "hr-cli")
		assertFormValue(t, r.Form, "scope", "openid offline_access hr.employee.read")
		assertFormValue(t, r.Form, "audience", "hr-gateway")
		if r.Form.Has("client_secret") {
			t.Fatal("public client request must not contain client_secret")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "device", "user_code": "ABCD-EFGH",
			"verification_uri": "https://auth.example.test/device",
			"expires_in":       600, "interval": 5,
		})
	}))
	defer server.Close()

	client := newHydraClient()
	device, err := client.requestDeviceAuthorization(context.Background(), pendingDeviceAuthorization{
		DeviceAuthorizationEndpoint: server.URL + "/oauth2/device/auth",
		ClientID:                    "hr-cli",
		Audience:                    "hr-gateway",
		Scope:                       "openid offline_access hr.employee.read",
	})
	if err != nil {
		t.Fatalf("requestDeviceAuthorization() error = %v", err)
	}
	if device.DeviceCode != "device" || device.UserCode != "ABCD-EFGH" {
		t.Fatalf("unexpected device response: %+v", device)
	}
}

func TestRequestDeviceAuthorizationRejectsUnsafeVerificationURL(t *testing.T) {
	tests := []struct {
		name         string
		verification string
		complete     string
	}{
		{name: "insecure scheme", verification: "javascript:alert(1)"},
		{name: "cross origin complete URL", verification: "https://manager.example.test/device", complete: "https://evil.example.test/device?user_code=ABCD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"device_code": "device", "user_code": "ABCD-EFGH",
					"verification_uri": tt.verification, "verification_uri_complete": tt.complete,
					"expires_in": 600, "interval": 5,
				})
			}))
			defer server.Close()

			_, err := newHydraClient().requestDeviceAuthorization(context.Background(), pendingDeviceAuthorization{
				DeviceAuthorizationEndpoint: server.URL,
				ClientID:                    "hr-cli",
			})
			if err == nil || err.Subtype != "hydra_device_response_invalid" {
				t.Fatalf("requestDeviceAuthorization() error = %#v, want hydra_device_response_invalid", err)
			}
		})
	}
}

func TestHydraDiscoveryProvidesStandardEndpoints(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			t.Fatalf("path = %q, want discovery path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(hydraDiscovery{
			Issuer:                      server.URL,
			DeviceAuthorizationEndpoint: server.URL + "/oauth2/device/auth",
			TokenEndpoint:               server.URL + "/oauth2/token",
			RevocationEndpoint:          server.URL + "/oauth2/revoke",
		})
	}))
	defer server.Close()

	discovery, err := newHydraClient().discover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if discovery.DeviceAuthorizationEndpoint != server.URL+"/oauth2/device/auth" {
		t.Fatalf("device endpoint = %q", discovery.DeviceAuthorizationEndpoint)
	}
	if discovery.TokenEndpoint != server.URL+"/oauth2/token" {
		t.Fatalf("token endpoint = %q", discovery.TokenEndpoint)
	}
	if discovery.RevocationEndpoint != server.URL+"/oauth2/revoke" {
		t.Fatalf("revocation endpoint = %q", discovery.RevocationEndpoint)
	}
}

func TestHydraClientDoesNotFollowOAuthRedirects(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	_, _, err := newHydraClient().postOAuthForm(context.Background(), redirect.URL, url.Values{
		"refresh_token": {"secret-refresh-token"},
	})
	if err == nil || err.Subtype != "hydra_http_error" {
		t.Fatalf("postOAuthForm() error = %#v, want hydra_http_error", err)
	}
	if targetHits != 0 {
		t.Fatalf("redirect target hits = %d, want 0", targetHits)
	}
}

func TestResolveHydraConfigUsesDiscoveryEndpoints(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HR_AUTH_BASE_URL", "http://127.0.0.1:18080")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(hydraDiscovery{
			Issuer:                      server.URL,
			DeviceAuthorizationEndpoint: server.URL + "/oauth2/device/auth",
			TokenEndpoint:               server.URL + "/oauth2/token",
			RevocationEndpoint:          server.URL + "/oauth2/revoke",
		})
	}))
	defer server.Close()

	config, err := resolveHydraConfig(context.Background(), newHydraClient(), appruntime.Profile{
		HydraIssuer: server.URL,
	}, "")
	if err != nil {
		t.Fatalf("resolveHydraConfig() error = %v", err)
	}
	if config.DeviceAuthorizationEndpoint != server.URL+"/oauth2/device/auth" ||
		config.TokenEndpoint != server.URL+"/oauth2/token" ||
		config.RevocationEndpoint != server.URL+"/oauth2/revoke" {
		t.Fatalf("unexpected discovered endpoints: %+v", config)
	}
	if config.ClientID != "hr-cli" || config.Audience != "hr-gateway" || !strings.Contains(config.Scope, "offline_access") {
		t.Fatalf("unexpected Hydra client contract: %+v", config)
	}
}

func TestVerificationURICompleteIsPreferred(t *testing.T) {
	pending := pendingDeviceAuthorization{
		VerificationURI:         "https://manager.example.test/device",
		VerificationURIComplete: "https://manager.example.test/device?user_code=ABCD",
	}
	if got := firstNonEmpty(pending.VerificationURIComplete, pending.VerificationURI); got != pending.VerificationURIComplete {
		t.Fatalf("preferred verification URI = %q, want complete URI", got)
	}
}

func TestPollDeviceTokenPendingSlowDownSuccess(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header = %q, want empty", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		assertFormValue(t, r.Form, "grant_type", deviceGrantType)
		assertFormValue(t, r.Form, "device_code", "device-code")
		assertFormValue(t, r.Form, "client_id", "hr-cli")
		if r.Form.Has("client_secret") {
			t.Fatal("device token request must not contain client_secret")
		}
		switch requests {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: "authorization_pending"})
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: "slow_down"})
		default:
			_ = json.NewEncoder(w).Encode(hydraTokenResponse{
				AccessToken: "access", RefreshToken: "refresh", TokenType: "bearer", ExpiresIn: 900,
			})
		}
	}))
	defer server.Close()

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	var sleeps []time.Duration
	client := newHydraClient()
	client.now = func() time.Time { return now }
	client.sleep = func(_ context.Context, duration time.Duration) error {
		sleeps = append(sleeps, duration)
		now = now.Add(duration)
		return nil
	}
	token, err := client.pollDeviceToken(context.Background(), pendingDeviceAuthorization{
		TokenEndpoint: server.URL,
		ClientID:      "hr-cli",
		DeviceCode:    "device-code",
		ExpiresAt:     now.Add(time.Minute).Format(time.RFC3339),
		Interval:      2,
	}, time.Minute)
	if err != nil {
		t.Fatalf("pollDeviceToken() error = %v", err)
	}
	if token.AccessToken != "access" || token.RefreshToken != "refresh" {
		t.Fatalf("unexpected token: %+v", token)
	}
	wantSleeps := []time.Duration{2 * time.Second, 2 * time.Second, 7 * time.Second}
	if len(sleeps) != len(wantSleeps) {
		t.Fatalf("sleep count = %d, want %d (%v)", len(sleeps), len(wantSleeps), sleeps)
	}
	for i := range wantSleeps {
		if sleeps[i] != wantSleeps[i] {
			t.Fatalf("sleep[%d] = %s, want %s", i, sleeps[i], wantSleeps[i])
		}
	}
}

func TestPollDeviceTokenWithoutTimeoutUsesDeviceExpiry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(hydraTokenResponse{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer", ExpiresIn: 900})
	}))
	defer server.Close()

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	client := newHydraClient()
	client.now = func() time.Time { return now }
	client.sleep = func(_ context.Context, duration time.Duration) error {
		now = now.Add(duration)
		return nil
	}
	token, err := client.pollDeviceToken(context.Background(), pendingDeviceAuthorization{
		TokenEndpoint: server.URL, ClientID: "hr-cli", DeviceCode: "device",
		ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339), Interval: 100,
	}, 0)
	if err != nil {
		t.Fatalf("pollDeviceToken() error = %v", err)
	}
	if token.AccessToken != "access" || requests != 3 {
		t.Fatalf("token = %+v, requests = %d; want success after 300 seconds", token, requests)
	}
}

func TestPollDeviceTokenExplicitTimeoutCapsPolling(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: "authorization_pending"})
	}))
	defer server.Close()

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	client := newHydraClient()
	client.now = func() time.Time { return now }
	client.sleep = func(_ context.Context, duration time.Duration) error {
		now = now.Add(duration)
		return nil
	}
	_, err := client.pollDeviceToken(context.Background(), pendingDeviceAuthorization{
		TokenEndpoint: server.URL, ClientID: "hr-cli", DeviceCode: "device",
		ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339), Interval: 60,
	}, 90*time.Second)
	if err == nil || err.Subtype != "login_timeout" {
		t.Fatalf("pollDeviceToken() error = %#v, want login_timeout", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 before explicit timeout cap", requests)
	}
}

func TestPollDeviceTokenAccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: "access_denied", ErrorDescription: "user denied"})
	}))
	defer server.Close()

	now := time.Now()
	client := newHydraClient()
	client.now = func() time.Time { return now }
	client.sleep = func(context.Context, time.Duration) error { return nil }
	_, err := client.pollDeviceToken(context.Background(), pendingDeviceAuthorization{
		TokenEndpoint: server.URL, ClientID: "hr-cli", DeviceCode: "device",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), Interval: 1,
	}, time.Minute)
	if err == nil || err.Subtype != "access_denied" {
		t.Fatalf("pollDeviceToken() error = %#v, want access_denied", err)
	}
}

func TestPollDeviceTokenTemporaryFailureRemainsResumable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: "temporarily_unavailable", ErrorDescription: "try again later"})
	}))
	defer server.Close()

	now := time.Now()
	client := newHydraClient()
	client.now = func() time.Time { return now }
	client.sleep = func(context.Context, time.Duration) error { return nil }
	_, err := client.pollDeviceToken(context.Background(), pendingDeviceAuthorization{
		TokenEndpoint: server.URL, ClientID: "hr-cli", DeviceCode: "device",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), Interval: 1,
	}, time.Minute)
	if err == nil || err.Type != "network" || err.Subtype != "hydra_token_unavailable" {
		t.Fatalf("pollDeviceToken() error = %#v, want network/hydra_token_unavailable", err)
	}
}

func TestPollDeviceTokenRequiresBearerAndRefreshToken(t *testing.T) {
	tests := []hydraTokenResponse{
		{AccessToken: "access", TokenType: "Bearer", ExpiresIn: 900},
		{AccessToken: "access", RefreshToken: "refresh", TokenType: "MAC", ExpiresIn: 900},
		{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer"},
	}
	for _, response := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(response)
		}))
		now := time.Now()
		client := newHydraClient()
		client.now = func() time.Time { return now }
		client.sleep = func(context.Context, time.Duration) error { return nil }
		_, err := client.pollDeviceToken(context.Background(), pendingDeviceAuthorization{
			TokenEndpoint: server.URL, ClientID: "hr-cli", DeviceCode: "device",
			ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), Interval: 1,
		}, time.Minute)
		server.Close()
		if err == nil || err.Subtype != "hydra_token_response_invalid" {
			t.Fatalf("pollDeviceToken(%+v) error = %#v, want hydra_token_response_invalid", response, err)
		}
	}
}

func TestRefreshTokenUsesPublicClientAndParsesRotation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header = %q, want empty", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		assertFormValue(t, r.Form, "grant_type", "refresh_token")
		assertFormValue(t, r.Form, "refresh_token", "old-refresh")
		assertFormValue(t, r.Form, "client_id", "hr-cli")
		if r.Form.Has("client_secret") {
			t.Fatal("refresh request must not contain client_secret")
		}
		_ = json.NewEncoder(w).Encode(hydraTokenResponse{
			AccessToken: "new-access", RefreshToken: "new-refresh", TokenType: "bearer", ExpiresIn: 900,
		})
	}))
	defer server.Close()

	client := newHydraClient()
	token, oauthErr, err := client.refreshToken(context.Background(), server.URL, "hr-cli", "old-refresh")
	if err != nil || oauthErr != nil {
		t.Fatalf("refreshToken() err = %v, oauthErr = %+v", err, oauthErr)
	}
	if token.AccessToken != "new-access" || token.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected refresh response: %+v", token)
	}
}

func TestRefreshHydraSessionHandlesTokenRemovedWhileWaitingForLock(t *testing.T) {
	t.Chdir(t.TempDir())
	session := appruntime.Session{
		Provider: ProviderHydra, Issuer: "https://auth-refresh-lock.example.test", ClientID: "hr-cli",
		AuthBaseURL: "https://gateway.example.test", TokenURL: "https://auth-refresh-lock.example.test/oauth2/token",
	}
	stored := StoredToken{
		Provider: ProviderHydra, AccessToken: "access", RefreshToken: "refresh",
		AccessTokenExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339),
	}
	defer func() { _ = removeStoredToken(session) }()
	if err := storeStoredToken(session, &stored); err != nil {
		t.Fatalf("storeStoredToken() error = %v", err)
	}
	lock, lockErr := acquireTokenLock(session, time.Second)
	if lockErr != nil {
		t.Fatalf("acquireTokenLock() error = %v", lockErr)
	}

	result := make(chan *errs.Error, 1)
	go func() {
		_, err := refreshHydraSessionIfNeeded(context.Background(), session, false)
		result <- err
	}()
	time.Sleep(250 * time.Millisecond)
	if err := removeStoredToken(session); err != nil {
		lock.release()
		t.Fatalf("removeStoredToken() error = %v", err)
	}
	lock.release()

	select {
	case err := <-result:
		if err == nil || err.Subtype != "missing_token" {
			t.Fatalf("refreshHydraSessionIfNeeded() error = %#v, want missing_token", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("refreshHydraSessionIfNeeded() did not return after lock release")
	}
}

func TestRevokeTokenUsesRFC7009PublicClientShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header = %q, want empty", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		assertFormValue(t, r.Form, "token", "refresh")
		assertFormValue(t, r.Form, "token_type_hint", "refresh_token")
		assertFormValue(t, r.Form, "client_id", "hr-cli")
		if r.Form.Has("client_secret") {
			t.Fatal("revocation request must not contain client_secret")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := newHydraClient().revokeToken(context.Background(), server.URL, "hr-cli", "refresh", "refresh_token"); err != nil {
		t.Fatalf("revokeToken() error = %v", err)
	}
}

func TestRevokeHydraSessionReportsOnlyCompleteRemoteRevocation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("token_type_hint") == "refresh_token" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	session := appruntime.Session{
		Provider: ProviderHydra, Issuer: "https://auth-revoke.example.test", ClientID: "hr-cli",
		AuthBaseURL: "https://gateway.example.test", RevocationURL: server.URL,
	}
	stored := StoredToken{Provider: ProviderHydra, AccessToken: "access", RefreshToken: "refresh"}
	defer func() { _ = removeStoredToken(session) }()
	if err := storeStoredToken(session, &stored); err != nil {
		t.Fatalf("storeStoredToken() error = %v", err)
	}
	if revoked := revokeHydraSession(context.Background(), session); revoked {
		t.Fatal("revokeHydraSession() = true, want false when refresh-token revocation failed")
	}
	if requests != 2 {
		t.Fatalf("revocation requests = %d, want 2", requests)
	}
}

func TestHydraTokenKeychainRoundTrip(t *testing.T) {
	resumeSuffix := strings.ReplaceAll(t.Name(), "/", "_")
	session := appruntime.Session{
		Provider: ProviderHydra, Issuer: "https://auth-" + resumeSuffix + ".example.test",
		ClientID: "hr-cli", AuthBaseURL: "https://gateway.example.test",
	}
	stored := StoredToken{
		Provider: ProviderHydra, Issuer: session.Issuer, ClientID: session.ClientID,
		AccessToken: "access", RefreshToken: "refresh",
		AccessTokenExpiresAt:  time.Now().Add(time.Hour).Format(time.RFC3339),
		RefreshTokenExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}
	defer func() { _ = removeStoredToken(session) }()
	if err := storeStoredToken(session, &stored); err != nil {
		t.Fatalf("storeStoredToken() error = %v", err)
	}
	loaded, err := loadStoredToken(session)
	if err != nil {
		t.Fatalf("loadStoredToken() error = %v", err)
	}
	if loaded == nil || loaded.AccessToken != "access" || loaded.RefreshToken != "refresh" {
		t.Fatalf("unexpected stored token: %+v", loaded)
	}
}

func TestPendingDeviceAuthorizationKeychainRoundTrip(t *testing.T) {
	resumeID, randomErr := randomResumeID()
	if randomErr != nil {
		t.Fatalf("randomResumeID() error = %v", randomErr)
	}
	pending := pendingDeviceAuthorization{
		ResumeID: "test-" + resumeID, DeviceCode: "device-secret", UserCode: "ABCD-EFGH",
		VerificationURI: "https://manager.example.test/device",
		ExpiresAt:       time.Now().Add(time.Minute).Format(time.RFC3339),
	}
	defer func() { _ = removePendingDeviceAuthorization(pending.ResumeID) }()
	if err := savePendingDeviceAuthorization(pending); err != nil {
		t.Fatalf("savePendingDeviceAuthorization() error = %v", err)
	}
	loaded, err := loadPendingDeviceAuthorization(pending.ResumeID)
	if err != nil {
		t.Fatalf("loadPendingDeviceAuthorization() error = %v", err)
	}
	if loaded.DeviceCode != pending.DeviceCode || loaded.UserCode != pending.UserCode {
		t.Fatalf("unexpected pending authorization: %+v", loaded)
	}
}

func TestLegacyProviderRemainsDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HR_AUTH_PROVIDER", "")
	provider, err := resolveLoginProvider(LoginRequest{})
	if err != nil {
		t.Fatalf("resolveLoginProvider() error = %v", err)
	}
	if provider != ProviderLegacyGateway {
		t.Fatalf("provider = %q, want %q", provider, ProviderLegacyGateway)
	}
	legacy := appruntime.Session{Source: "dingtalk_oauth", AuthBaseURL: "https://gateway.example.test", EID: "94"}
	if got := tokenAccount(legacy); !strings.HasPrefix(got, "dingtalk:") {
		t.Fatalf("legacy token account = %q, want dingtalk prefix", got)
	}
}

func TestResolveLoginProviderRejectsMixedResumeFlags(t *testing.T) {
	t.Chdir(t.TempDir())
	tests := []LoginRequest{
		{ResumeID: "resume", DingTalk: true},
		{Provider: ProviderHydra, LoginID: "legacy-id", LoginSecret: "legacy-secret"},
		{ResumeID: "resume", NoWait: true},
	}
	for _, req := range tests {
		if _, err := resolveLoginProvider(req); err == nil || err.Type != "validation" {
			t.Fatalf("resolveLoginProvider(%+v) error = %#v, want validation error", req, err)
		}
	}
}

func TestHydraTokenAccountIncludesIssuerPath(t *testing.T) {
	root := tokenAccount(appruntime.Session{Provider: ProviderHydra, Issuer: "https://auth.example.test", ClientID: "hr-cli"})
	nested := tokenAccount(appruntime.Session{Provider: ProviderHydra, Issuer: "https://auth.example.test/hydra", ClientID: "hr-cli"})
	if root == nested {
		t.Fatalf("token accounts collide for distinct issuers: %q", root)
	}
}

func TestTokenWithoutRefreshRemainsUsableUntilAccessExpiry(t *testing.T) {
	token := &StoredToken{AccessTokenExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339)}
	if got := tokenStatus(token); got != "valid" {
		t.Fatalf("tokenStatus() = %q, want valid", got)
	}
}

func TestSecureOAuthURLAllowsOnlyHTTPSOrLocalHTTP(t *testing.T) {
	tests := map[string]bool{
		"https://auth.example.test":        true,
		"http://localhost:4444":            true,
		"http://127.0.0.1:4444":            true,
		"http://[::1]:4444":                true,
		"https://secret@auth.example.test": false,
		"http://auth.example.test":         false,
		"ftp://auth.example.test":          false,
	}
	for endpoint, want := range tests {
		if got := isSecureOAuthURL(endpoint); got != want {
			t.Errorf("isSecureOAuthURL(%q) = %v, want %v", endpoint, got, want)
		}
	}
}

func assertFormValue(t *testing.T, form url.Values, key, want string) {
	t.Helper()
	if got := form.Get(key); got != want {
		t.Fatalf("form[%q] = %q, want %q", key, got, want)
	}
}
