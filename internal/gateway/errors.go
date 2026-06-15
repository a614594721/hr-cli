// Package gateway is the hr-cli HTTP client for hr-gateway.
//
// All business commands send requests through this package; no command may
// reach a database or implement perm/scope/audit on its own. The thin-client
// boundary is enforced here.
//
// See docs/hr-cli-architecture-credential-isolation.md for the full design.
package gateway

import (
	"encoding/json"
	"fmt"

	"hr-cli/internal/errs"
)

// envelope is the wire-format of a gateway error response body.
//
// Successful responses are not parsed through this struct — callers pass an
// `out` value to Do() and the success payload is decoded directly into it.
type envelope struct {
	OK    bool         `json:"ok"`
	Error envelopeBody `json:"error"`
}

type envelopeBody struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// parseError decodes a gateway error envelope into *errs.Error.
//
// Falls back to a synthetic network error when the body is empty, malformed,
// or missing required fields — gateway is the only system the client trusts
// to produce well-formed envelopes, so anything else is treated as a transport
// fault, not as a "this is fine but unparseable" success.
func parseError(status int, body []byte) *errs.Error {
	if len(body) == 0 {
		return errs.Network("gateway_empty_response",
			fmt.Sprintf("gateway returned HTTP %d with empty body", status))
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return errs.Network("gateway_bad_envelope",
			fmt.Sprintf("gateway returned HTTP %d with non-envelope body: %s", status, truncate(body, 200)))
	}
	if env.OK {
		return errs.Network("gateway_unexpected_ok",
			fmt.Sprintf("gateway returned HTTP %d but envelope ok=true", status))
	}
	if env.Error.Type == "" {
		return errs.Network("gateway_bad_envelope",
			fmt.Sprintf("gateway returned HTTP %d with empty error.type", status))
	}
	out := &errs.Error{
		Type:     env.Error.Type,
		Subtype:  env.Error.Subtype,
		Message:  env.Error.Message,
		Param:    env.Error.Param,
		Hint:     env.Error.Hint,
		ExitCode: exitCodeFor(env.Error.Type),
	}
	return out
}

// exitCodeFor mirrors the exit-code policy in internal/errs so that errors
// surfaced by the gateway exit with the same code as locally-raised errors of
// the same type.
func exitCodeFor(typeName string) int {
	switch typeName {
	case "validation", "config":
		return 2
	case "authentication", "authorization", "policy", "confirmation":
		return 3
	case "db", "network":
		return 4
	case "internal":
		return 5
	default:
		return 1
	}
}

func truncate(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "..."
}

// IsTokenExpired reports whether an error indicates the access token has
// expired and should be refreshed. Used by Do() to drive the refresh-once
// retry loop.
func IsTokenExpired(err *errs.Error) bool {
	return err != nil && err.Type == "authentication" && err.Subtype == "token_expired"
}

// IsRefreshInvalid reports whether the refresh token itself is no longer
// usable. Callers should surface a re-login prompt rather than retry.
func IsRefreshInvalid(err *errs.Error) bool {
	return err != nil && err.Type == "authentication" && err.Subtype == "refresh_invalid"
}
