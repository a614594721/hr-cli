package gateway

import (
	"context"
	"net/http"
	"time"

	"hr-cli/internal/errs"
	"hr-cli/internal/runtime"
)

// FromActiveProfile constructs a Client using the auth_base_url stored on
// the active hr-cli profile. Returns config/gateway_url_missing when no
// profile is active or the URL is empty so every command surfaces the same
// hint.
func FromActiveProfile(opts ...Option) (*Client, *errs.Error) {
	profile, ok := runtime.ActiveProfile()
	var base string
	if ok {
		base = profile.AuthBaseURL
	}
	return New(base, DefaultTokenSource(), opts...)
}

// writePaths lists the apply / write endpoints that need both the longer
// timeout and the X-HR-Confirm header. Kept as a small map (not regex) so
// adding a new write path is a one-line change and the list reads as the
// route table.
var writePaths = map[string]bool{
	"/api/hr-cli/v1/transfer/apply":     true,
	"/api/hr-cli/v1/profile-info/apply": true,
	"/api/hr-cli/v1/approval/write":     true,
}

// Call is the one-shot helper used by every business command's
// `--via-gateway` branch. write=true sets X-HR-Confirm: yes; callers should
// only pass true when the user supplied --yes or otherwise confirmed.
//
// The decoded body is returned as map[string]any so the caller can pass it
// straight to emit(). Commands that need a typed payload should call Do()
// on the underlying Client directly.
func Call(ctx context.Context, method, path string, body any, write bool) (map[string]any, *errs.Error) {
	var opts []Option
	if write {
		opts = append(opts, WithConfirm())
	}
	if writePaths[path] {
		opts = append(opts, WithTimeout(60*time.Second))
	}
	c, err := FromActiveProfile(opts...)
	if err != nil {
		return nil, err
	}
	if method == "" {
		method = http.MethodPost
	}
	var out map[string]any
	if e := c.Do(ctx, method, path, body, &out); e != nil {
		return nil, e
	}
	return out, nil
}
