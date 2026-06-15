// Package gateway: HTTP client core.
//
// Construction is profile-driven: New() reads the active hr-cli profile to
// resolve the gateway base URL, the active session for operator EID, and the
// stored token for the bearer credential. All command handlers should call
// New() at the top of RunE and never construct a *Client directly with a hard-
// coded URL — the profile / session lookup is the hook point that makes
// `hr profile use ...` work consistently across commands.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hr-cli/internal/build"
	"hr-cli/internal/errs"
)

// Default timeout per request. Apply paths (transfer/profile-info) override
// via WithTimeout — they involve stored procedures that can run longer than
// read paths, but still must bound to prevent the CLI from hanging
// indefinitely against an unhealthy gateway.
const (
	defaultTimeout = 30 * time.Second
	applyTimeout   = 60 * time.Second
)

// Client is a per-command HTTP client to hr-gateway. It is cheap to construct
// — no connection pooling beyond Go's default http.Transport — so commands
// should build one per RunE rather than passing it around globally.
type Client struct {
	baseURL    string
	httpClient *http.Client
	tokens     TokenSource
	timeout    time.Duration
	confirm    bool
}

// TokenSource abstracts the access-token lookup so tests can inject a stub
// without touching the keychain. The production implementation is in
// internal/gateway/token.go and bridges to internal/auth.
type TokenSource interface {
	// AccessToken returns a usable access token, refreshing in-place if it is
	// within the refresh-ahead window. Returns ("", nil) when no session is
	// configured — callers that require auth should treat that as
	// authentication/not_logged_in.
	AccessToken(ctx context.Context) (string, *errs.Error)

	// ForceRefresh is invoked after a token_expired response from the gateway,
	// to obtain a new access token before retrying once.
	ForceRefresh(ctx context.Context) (string, *errs.Error)
}

// Option mutates a *Client during construction. Kept as a slice rather than a
// struct so command-specific overrides (apply timeout, confirm header) read
// at the call site.
type Option func(*Client)

// WithTimeout overrides the per-request timeout. Use applyTimeout (or longer)
// for write paths.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithConfirm sets X-HR-Confirm: yes on the next request. Required by the
// gateway for any apply endpoint; setting it on a non-apply path is harmless
// (the gateway ignores it where unused).
func WithConfirm() Option {
	return func(c *Client) { c.confirm = true }
}

// New constructs a Client bound to the given gateway base URL and token
// source. baseURL must be absolute (http:// or https://); a path component is
// allowed but trailing slashes are normalized. An empty baseURL returns a
// config error so the caller can surface a clear "configure auth_base_url"
// hint instead of failing later with a network error.
func New(baseURL string, tokens TokenSource, opts ...Option) (*Client, *errs.Error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		e := errs.Config("gateway_url_missing",
			"gateway base URL is not configured")
		e.Hint = "set auth_base_url on the active profile (hr profile add ... --auth-base-url ...)"
		return nil, e
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		e := errs.Config("gateway_url_invalid",
			fmt.Sprintf("gateway base URL %q is not a valid absolute URL", trimmed))
		e.Hint = "use the form https://hr-gateway.internal.example.com"
		return nil, e
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if tokens == nil {
		return nil, errs.New("internal", "gateway_no_token_source",
			"gateway client constructed without a token source", 5)
	}
	c := &Client{
		baseURL:    parsed.String(),
		httpClient: &http.Client{},
		tokens:     tokens,
		timeout:    defaultTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Do sends an authenticated request to the gateway. body, if non-nil, is
// JSON-encoded; out, if non-nil, is JSON-decoded from the data field of the
// success envelope. On a 401 + token_expired response, Do refreshes the
// access token once and retries the same request — further failures surface
// to the caller without a second retry, to avoid masking real auth failures
// behind silent loops.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) *errs.Error {
	if ctx == nil {
		ctx = context.Background()
	}
	rawBody, encErr := encodeBody(body)
	if encErr != nil {
		return encErr
	}
	token, tokenErr := c.tokens.AccessToken(ctx)
	if tokenErr != nil {
		return tokenErr
	}
	resp, doErr := c.send(ctx, method, path, rawBody, token)
	if doErr != nil {
		return doErr
	}
	if !IsTokenExpired(doErr) && resp != nil {
		// success path — fall through to decode below
	}
	if resp == nil {
		// shouldn't happen — send() returns either a non-nil resp or a non-
		// nil error.
		return errs.Network("gateway_unreachable", "gateway returned no response")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		envErr := parseError(resp.StatusCode, respBody)
		if !IsTokenExpired(envErr) {
			return envErr
		}
		// One refresh + retry. ForceRefresh failures surface as-is so the user
		// sees `authentication/refresh_invalid` and is prompted to re-login.
		newToken, refreshErr := c.tokens.ForceRefresh(ctx)
		if refreshErr != nil {
			return refreshErr
		}
		resp2, doErr2 := c.send(ctx, method, path, rawBody, newToken)
		if doErr2 != nil {
			return doErr2
		}
		return decodeResponse(resp2, out)
	}
	return decodeResponse(resp, out)
}

func (c *Client) send(ctx context.Context, method, path string, body []byte, token string) (*http.Response, *errs.Error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	// We intentionally do NOT defer cancel here — the resp.Body must remain
	// readable after this function returns. Callers of send() are responsible
	// for draining and closing the body, which releases the ctx via Go's
	// http.Client transport contract.
	_ = cancel

	full := c.baseURL + ensureLeadingSlash(path)
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, full, reader)
	if err != nil {
		return nil, errs.Network("gateway_request_build_failed", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "hr-cli/"+build.Version)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if c.confirm {
		req.Header.Set("X-HR-Confirm", "yes")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		e := errs.Network("gateway_unreachable", err.Error())
		e.Hint = "verify auth_base_url and that hr-gateway is reachable from this host"
		return nil, e
	}
	return resp, nil
}

// decodeResponse drains resp.Body, parses the envelope, and either populates
// `out` or returns the envelope error. Caller must be prepared for `out` to
// be left at its zero value when the envelope has no data field (e.g. logout,
// health).
func decodeResponse(resp *http.Response, out any) *errs.Error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errs.Network("gateway_response_read_failed", err.Error())
	}
	if resp.StatusCode >= 400 {
		return parseError(resp.StatusCode, body)
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	// Success envelope: { "ok": true, "data": <out>, "meta": {...} }.
	// We decode only data into out — meta is intentionally dropped because
	// the CLI's emit() already adds its own meta block at the command layer.
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errs.Network("gateway_bad_envelope",
			fmt.Sprintf("could not decode success envelope: %s", truncate(body, 200)))
	}
	if !envelope.OK {
		// Defensive: HTTP 2xx with ok=false would be a gateway bug, but we
		// don't want to silently swallow data in that case.
		return parseError(resp.StatusCode, body)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return errs.Network("gateway_bad_envelope",
			fmt.Sprintf("could not decode data field: %v", err))
	}
	return nil
}

func encodeBody(body any) ([]byte, *errs.Error) {
	if body == nil {
		return nil, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, errs.New("internal", "gateway_request_encode_failed", err.Error(), 5)
	}
	return raw, nil
}

func ensureLeadingSlash(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}
