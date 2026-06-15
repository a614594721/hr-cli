package gateway

import (
	"context"

	"hr-cli/internal/auth"
	"hr-cli/internal/errs"
)

// dingtalkTokenSource bridges the gateway client to the DingTalk OAuth broker
// token store implemented in internal/auth. Kept inside the gateway package
// so that internal/auth need not import internal/gateway and so that tests
// can swap in a fake by satisfying the TokenSource interface directly.
type dingtalkTokenSource struct{}

// DefaultTokenSource returns a TokenSource backed by the active session and
// keychain-stored DingTalk tokens. Returns
// authentication/not_logged_in when no DingTalk OAuth session is present —
// callers can treat that as a soft "please run auth +login --dingtalk".
func DefaultTokenSource() TokenSource {
	return dingtalkTokenSource{}
}

func (dingtalkTokenSource) AccessToken(_ context.Context) (string, *errs.Error) {
	token, err := auth.AccessToken(false)
	if err != nil {
		return "", err
	}
	if token == "" {
		e := errs.Authentication("not_logged_in",
			"no active DingTalk session")
		e.Hint = "run hr auth +login --dingtalk"
		return "", e
	}
	return token, nil
}

func (dingtalkTokenSource) ForceRefresh(_ context.Context) (string, *errs.Error) {
	token, err := auth.AccessToken(true)
	if err != nil {
		return "", err
	}
	if token == "" {
		e := errs.Authentication("not_logged_in",
			"no active DingTalk session")
		e.Hint = "run hr auth +login --dingtalk"
		return "", e
	}
	return token, nil
}
