package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"hr-cli/internal/errs"
	appruntime "hr-cli/internal/runtime"
)

type loginStartResponse struct {
	LoginID             string `json:"loginId"`
	LoginSecret         string `json:"loginSecret"`
	AuthURL             string `json:"authUrl"`
	ExpiresAt           string `json:"expiresAt"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
}

type tokenResponse struct {
	Status           string   `json:"status"`
	TokenType        string   `json:"tokenType"`
	AccessToken      string   `json:"accessToken"`
	ExpiresAt        string   `json:"expiresAt"`
	ExpiresIn        int      `json:"expiresIn"`
	RefreshToken     string   `json:"refreshToken"`
	RefreshExpiresAt string   `json:"refreshExpiresAt"`
	Operator         Operator `json:"operator"`
}

type remoteMeResponse struct {
	Operator  Operator `json:"operator"`
	ExpiresAt string   `json:"expiresAt"`
}

func LoginDingTalk(req LoginRequest) (map[string]any, *errs.Error) {
	baseURL, err := authBaseURL(req.AuthBaseURL)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	var started loginStartResponse
	if req.LoginID != "" || req.LoginSecret != "" {
		if req.LoginID == "" || req.LoginSecret == "" {
			return nil, errs.Validation("missing_login_resume_fields", "--login-id and --login-secret must be provided together")
		}
		started = loginStartResponse{LoginID: req.LoginID, LoginSecret: req.LoginSecret}
	} else {
		started, err = startRemoteLogin(baseURL)
		if err != nil {
			return nil, err
		}
		if req.NoWait {
			return map[string]any{
				"status":                "pending",
				"mode":                  "dingtalk_oauth",
				"login_id":              started.LoginID,
				"login_secret":          started.LoginSecret,
				"auth_url":              started.AuthURL,
				"expires_at":            started.ExpiresAt,
				"poll_interval_seconds": started.PollIntervalSeconds,
				"next":                  "open auth_url, then run auth +login --dingtalk --login-id <login_id> --login-secret <login_secret>",
			}, nil
		}
		if !req.NoBrowser {
			if openErr := openBrowser(started.AuthURL); openErr != nil {
				// Keep going: the returned auth_url is enough for manual login.
				fmt.Fprintf(os.Stderr, "failed to open browser automatically: %v\n", openErr)
			}
		}
		fmt.Fprintf(os.Stderr, "DingTalk login URL: %s\n", started.AuthURL)
	}
	token, err := pollRemoteLogin(baseURL, started, timeout)
	if err != nil {
		return nil, err
	}
	session := sessionFromToken(baseURL, token)
	if saveErr := storeToken(session, token); saveErr != nil {
		return nil, saveErr
	}
	if saveErr := appruntime.SaveSession(session); saveErr != nil {
		return nil, saveErr
	}
	return map[string]any{
		"status":             "active",
		"mode":               "dingtalk_oauth",
		"operator":           token.Operator,
		"access_expires_at":  token.ExpiresAt,
		"refresh_expires_at": token.RefreshExpiresAt,
	}, nil
}

func authBaseURL(flagValue string) (string, *errs.Error) {
	profile, _ := appruntime.ActiveProfile()
	value := firstNonEmpty(flagValue, os.Getenv("HR_AUTH_BASE_URL"), os.Getenv("HR_CLI_AUTH_BASE_URL"), profile.AuthBaseURL)
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		e := errs.Config("missing_auth_base_url", "DingTalk login requires --auth-base-url, HR_AUTH_BASE_URL, or profile auth_base_url")
		e.Param = "--auth-base-url"
		return "", e
	}
	parsed, parseErr := url.Parse(value)
	if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		e := errs.Validation("invalid_auth_base_url", "auth base URL must be an absolute http(s) URL")
		e.Param = "--auth-base-url"
		return "", e
	}
	return value, nil
}

func startRemoteLogin(baseURL string) (loginStartResponse, *errs.Error) {
	var result loginStartResponse
	err := remoteJSON(http.MethodPost, baseURL+"/api/hr-cli/auth/login/start", "", nil, &result)
	if err != nil {
		return result, err
	}
	if result.LoginID == "" || result.LoginSecret == "" || result.AuthURL == "" {
		return result, errs.Network("invalid_login_start_response", "auth broker returned an incomplete login start response")
	}
	return result, nil
}

func pollRemoteLogin(baseURL string, started loginStartResponse, timeout time.Duration) (tokenResponse, *errs.Error) {
	deadline := time.Now().Add(timeout)
	interval := time.Duration(started.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		var result tokenResponse
		pollURL := baseURL + "/api/hr-cli/auth/login/poll?" + url.Values{
			"loginId":     []string{started.LoginID},
			"loginSecret": []string{started.LoginSecret},
		}.Encode()
		err := remoteJSON(http.MethodGet, pollURL, "", nil, &result)
		if err == nil && result.Status == "succeeded" {
			return result, nil
		}
		if err != nil {
			return result, err
		}
		if time.Now().After(deadline) {
			return result, errs.Network("login_timeout", "DingTalk login timed out")
		}
		time.Sleep(interval)
	}
}

func refreshSessionIfNeeded(session appruntime.Session, force bool) (appruntime.Session, *errs.Error) {
	if session.Source != "dingtalk_oauth" || session.AuthBaseURL == "" || session.EID == "" {
		return session, nil
	}
	stored, err := loadStoredToken(session)
	if err != nil {
		return session, err
	}
	status := tokenStatus(stored)
	if status == "missing" {
		return session, errs.Authentication("missing_token", "DingTalk token is missing; run auth +login --dingtalk")
	}
	if status == "expired" {
		_ = removeStoredToken(session)
		return session, errs.Authentication("refresh_token_expired", "DingTalk refresh token expired; run auth +login --dingtalk")
	}
	if !force && status == "valid" {
		return session, nil
	}

	lock, err := acquireTokenLock(session, 30*time.Second)
	if err != nil {
		return session, err
	}
	defer lock.release()

	stored, err = loadStoredToken(session)
	if err != nil {
		return session, err
	}
	status = tokenStatus(stored)
	if !force && status == "valid" {
		return sessionFromStoredToken(session, stored), nil
	}
	if status == "expired" || status == "missing" {
		_ = removeStoredToken(session)
		return session, errs.Authentication("refresh_token_expired", "DingTalk refresh token expired; run auth +login --dingtalk")
	}

	refreshed, token, err := refreshRemoteSession(session, stored)
	if err != nil {
		if shouldClearTokenAfterRefreshError(err) {
			_ = removeStoredToken(session)
		}
		return session, err
	}
	if saveErr := storeToken(refreshed, token); saveErr != nil {
		return session, saveErr
	}
	if saveErr := appruntime.SaveSession(refreshed); saveErr != nil {
		return session, saveErr
	}
	return refreshed, nil
}

func refreshRemoteSession(session appruntime.Session, stored *StoredToken) (appruntime.Session, tokenResponse, *errs.Error) {
	body := map[string]string{"refreshToken": stored.RefreshToken}
	var token tokenResponse
	err := remoteJSON(http.MethodPost, session.AuthBaseURL+"/api/hr-cli/auth/refresh", "", body, &token)
	if err != nil {
		return session, token, err
	}
	return sessionFromToken(session.AuthBaseURL, token), token, nil
}

func shouldClearTokenAfterRefreshError(err *errs.Error) bool {
	return err != nil && err.Type == "authentication"
}

func remoteMe(session appruntime.Session, stored *StoredToken) (Operator, string, *errs.Error) {
	// /v1/auth/me is a business endpoint and returns the gateway envelope
	// {ok,data,meta}, unlike the /auth/login/* broker paths which are flat.
	// remoteJSON is envelope-unaware, so unwrap manually.
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := remoteJSON(http.MethodGet, session.AuthBaseURL+"/api/hr-cli/v1/auth/me", stored.AccessToken, nil, &envelope); err != nil {
		return Operator{}, "", err
	}
	if !envelope.OK || len(envelope.Data) == 0 {
		return Operator{}, "", errs.Network("invalid_me_response", "gateway /auth/me returned no data")
	}
	var result remoteMeResponse
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return Operator{}, "", errs.Network("invalid_me_response", err.Error())
	}
	return result.Operator, result.ExpiresAt, nil
}

func revokeRemoteSession(session appruntime.Session) bool {
	stored, err := loadStoredToken(session)
	if err != nil || stored == nil || stored.RefreshToken == "" {
		return false
	}
	body := map[string]string{"refreshToken": stored.RefreshToken}
	var result map[string]any
	return remoteJSON(http.MethodPost, session.AuthBaseURL+"/api/hr-cli/auth/logout", "", body, &result) == nil
}

func sessionFromToken(baseURL string, token tokenResponse) appruntime.Session {
	return appruntime.Session{
		EID:                   token.Operator.EID,
		URID:                  token.Operator.URID,
		Badge:                 token.Operator.Badge,
		Name:                  token.Operator.Name,
		Role:                  token.Operator.Role,
		Source:                "dingtalk_oauth",
		AuthBaseURL:           baseURL,
		AccessTokenExpiresAt:  token.ExpiresAt,
		RefreshTokenExpiresAt: token.RefreshExpiresAt,
	}
}

func sessionFromStoredToken(session appruntime.Session, token *StoredToken) appruntime.Session {
	if token == nil {
		return session
	}
	session.AccessTokenExpiresAt = token.AccessTokenExpiresAt
	session.RefreshTokenExpiresAt = token.RefreshTokenExpiresAt
	return session
}

func remoteJSON(method, endpoint, bearer string, body any, out any) *errs.Error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			return errs.New("internal", "json_encode_failed", err.Error(), 5)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return errs.Network("request_create_failed", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return errs.Network("request_failed", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := errs.Network(fmt.Sprintf("remote_http_%d", resp.StatusCode), fmt.Sprintf("auth broker returned HTTP %d", resp.StatusCode))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			e.Type = "authentication"
			e.ExitCode = 3
		}
		return e
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errs.Network("response_decode_failed", err.Error())
	}
	return nil
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
