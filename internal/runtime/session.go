package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"hr-cli/internal/errs"
)

type Session struct {
	EID                   string `json:"eid,omitempty"`
	URID                  string `json:"urid,omitempty"`
	Badge                 string `json:"badge,omitempty"`
	Name                  string `json:"name"`
	Role                  string `json:"role"`
	Source                string `json:"source"`
	AuthBaseURL           string `json:"auth_base_url,omitempty"`
	AccessTokenExpiresAt  string `json:"access_token_expires_at,omitempty"`
	RefreshTokenExpiresAt string `json:"refresh_token_expires_at,omitempty"`
	CreatedAt             string `json:"created_at"`
}

func SaveSession(session Session) *errs.Error {
	dir := filepath.Dir(sessionPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Config("session_write_failed", err.Error())
	}
	if session.CreatedAt == "" {
		session.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return errs.Config("session_encode_failed", err.Error())
	}
	if err := os.WriteFile(sessionPath(), data, 0o600); err != nil {
		return errs.Config("session_write_failed", err.Error())
	}
	return nil
}

func LoadSession() (Session, bool) {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return Session{}, false
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, false
	}
	if session.Name == "" {
		return Session{}, false
	}
	if bytes.Contains(data, []byte(`"access_token"`)) || bytes.Contains(data, []byte(`"refresh_token"`)) {
		_ = SaveSession(session)
	}
	return session, true
}

func ClearSession() (bool, *errs.Error) {
	if err := os.Remove(sessionPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, errs.Config("session_clear_failed", err.Error())
	}
	return true, nil
}

func sessionPath() string {
	return filepath.Join(".", ".hr-cli", "session.json")
}
