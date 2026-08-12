package gateway

import (
	"context"

	"hr-cli/internal/auth"
	"hr-cli/internal/errs"
)

// sessionTokenSource bridges the gateway client to the active authentication
// provider implemented in internal/auth. Kept inside the gateway package
// so that internal/auth need not import internal/gateway and so that tests
// can swap in a fake by satisfying the TokenSource interface directly.
type sessionTokenSource struct{}

// DefaultTokenSource returns a TokenSource backed by the active session and
// keychain-stored provider tokens. It returns authentication/not_logged_in
// when no authentication session is present.
func DefaultTokenSource() TokenSource {
	return sessionTokenSource{}
}

func (sessionTokenSource) AccessToken(_ context.Context) (string, *errs.Error) {
	token, err := auth.AccessToken(false)
	if err != nil {
		return "", err
	}
	if token == "" {
		e := errs.Authentication("not_logged_in", "no active authentication session")
		e.Hint = "run hr auth +login"
		return "", e
	}
	return token, nil
}

func (sessionTokenSource) ForceRefresh(_ context.Context) (string, *errs.Error) {
	token, err := auth.AccessToken(true)
	if err != nil {
		return "", err
	}
	if token == "" {
		e := errs.Authentication("not_logged_in", "no active authentication session")
		e.Hint = "run hr auth +login"
		return "", e
	}
	return token, nil
}
