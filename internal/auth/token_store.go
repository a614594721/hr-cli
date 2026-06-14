package auth

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"hr-cli/internal/errs"
	"hr-cli/internal/keychain"
	appruntime "hr-cli/internal/runtime"
)

const refreshAhead = 5 * time.Minute

var safeTokenIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type StoredToken struct {
	AuthBaseURL           string `json:"authBaseUrl"`
	EID                   string `json:"eid"`
	AccessToken           string `json:"accessToken"`
	AccessTokenExpiresAt  string `json:"accessTokenExpiresAt"`
	RefreshToken          string `json:"refreshToken"`
	RefreshTokenExpiresAt string `json:"refreshTokenExpiresAt"`
	GrantedAt             string `json:"grantedAt"`
}

func tokenAccount(session appruntime.Session) string {
	return fmt.Sprintf("dingtalk:%s:%s", authBaseAccountKey(session.AuthBaseURL), normalizeAccountPart(session.EID))
}

func normalizeAccountPart(value string) string {
	value = safeTokenIDChars.ReplaceAllString(value, "_")
	if value == "" {
		return "default"
	}
	return value
}

func storeToken(session appruntime.Session, token tokenResponse) *errs.Error {
	stored := StoredToken{
		AuthBaseURL:           session.AuthBaseURL,
		EID:                   session.EID,
		AccessToken:           token.AccessToken,
		AccessTokenExpiresAt:  token.ExpiresAt,
		RefreshToken:          token.RefreshToken,
		RefreshTokenExpiresAt: token.RefreshExpiresAt,
		GrantedAt:             time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return errs.New("internal", "token_encode_failed", err.Error(), 5)
	}
	if err := keychain.Set(keychain.ServiceName, tokenAccount(session), string(data)); err != nil {
		return errs.Config("token_store_failed", err.Error())
	}
	return nil
}

func loadStoredToken(session appruntime.Session) (*StoredToken, *errs.Error) {
	data, err := keychain.Get(keychain.ServiceName, tokenAccount(session))
	if err != nil {
		return nil, errs.Config("token_load_failed", err.Error())
	}
	if data == "" {
		return nil, nil
	}
	var token StoredToken
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, errs.Config("token_parse_failed", err.Error())
	}
	return &token, nil
}

func removeStoredToken(session appruntime.Session) *errs.Error {
	if session.AuthBaseURL == "" || session.EID == "" {
		return nil
	}
	if err := keychain.Remove(keychain.ServiceName, tokenAccount(session)); err != nil {
		return errs.Config("token_remove_failed", err.Error())
	}
	return nil
}

func tokenStatus(token *StoredToken) string {
	if token == nil {
		return "missing"
	}
	now := time.Now()
	accessExpiresAt, accessOK := parseTokenTime(token.AccessTokenExpiresAt)
	refreshExpiresAt, refreshOK := parseTokenTime(token.RefreshTokenExpiresAt)
	if !refreshOK || !now.Before(refreshExpiresAt) {
		return "expired"
	}
	if !accessOK || now.After(accessExpiresAt.Add(-refreshAhead)) {
		return "needs_refresh"
	}
	return "valid"
}

func parseTokenTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, true
	}
	parsed, err = time.Parse("2006-01-02 15:04:05", value)
	if err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

type tokenLock struct {
	path string
	file *os.File
}

func acquireTokenLock(session appruntime.Session, timeout time.Duration) (*tokenLock, *errs.Error) {
	dir := filepath.Join(".", ".hr-cli", "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errs.Config("token_lock_failed", err.Error())
	}
	lockPath := filepath.Join(dir, normalizeAccountPart(tokenAccount(session))+".lock")
	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = file.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
			return &tokenLock{path: lockPath, file: file}, nil
		}
		if !os.IsExist(err) {
			return nil, errs.Config("token_lock_failed", err.Error())
		}
		clearStaleLock(lockPath, 2*time.Minute)
		if time.Now().After(deadline) {
			return nil, errs.Config("token_lock_timeout", "timeout waiting for token refresh lock")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (lock *tokenLock) release() {
	if lock == nil {
		return
	}
	if lock.file != nil {
		_ = lock.file.Close()
	}
	_ = os.Remove(lock.path)
}

func clearStaleLock(path string, maxAge time.Duration) {
	info, err := os.Stat(path)
	if err == nil && time.Since(info.ModTime()) > maxAge {
		_ = os.Remove(path)
	}
}

func authBaseAccountKey(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return normalizeAccountPart(baseURL)
	}
	return normalizeAccountPart(parsed.Scheme + "://" + parsed.Host)
}
