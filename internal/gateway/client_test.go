package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"hr-cli/internal/errs"
)

type fakeTokens struct {
	access      string
	refreshed   string
	refreshFail *errs.Error
	getCount    int32
	refreshHits int32
}

func (f *fakeTokens) AccessToken(context.Context) (string, *errs.Error) {
	atomic.AddInt32(&f.getCount, 1)
	return f.access, nil
}

func (f *fakeTokens) ForceRefresh(context.Context) (string, *errs.Error) {
	atomic.AddInt32(&f.refreshHits, 1)
	if f.refreshFail != nil {
		return "", f.refreshFail
	}
	return f.refreshed, nil
}

func TestNew_validatesBaseURL(t *testing.T) {
	tokens := &fakeTokens{access: "t"}
	cases := []struct{ name, url, wantSubtype string }{
		{"empty", "", "gateway_url_missing"},
		{"whitespace", "   ", "gateway_url_missing"},
		{"no_scheme", "hr-gateway.local", "gateway_url_invalid"},
		{"no_host", "http://", "gateway_url_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.url, tokens)
			if err == nil || err.Subtype != tc.wantSubtype {
				t.Fatalf("got err=%v want subtype=%s", err, tc.wantSubtype)
			}
		})
	}
}

func TestNew_requiresTokenSource(t *testing.T) {
	_, err := New("https://gw.example.com", nil)
	if err == nil || err.Subtype != "gateway_no_token_source" {
		t.Fatalf("expected gateway_no_token_source, got %v", err)
	}
}

func TestNew_normalizesTrailingSlash(t *testing.T) {
	c, err := New("https://gw.example.com/", &fakeTokens{access: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(c.baseURL, "/") {
		t.Fatalf("expected trailing slash trimmed, got %q", c.baseURL)
	}
}

func TestDo_decodesSuccessData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer initial" {
			t.Errorf("missing/wrong Authorization header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type=%q want application/json", got)
		}
		if got := r.Header.Get("X-HR-Confirm"); got != "" {
			t.Errorf("unexpected X-HR-Confirm=%q without WithConfirm()", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"data":{"name":"wubang","eid":"94"}}`)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, &fakeTokens{access: "initial"})
	var out struct {
		Name string `json:"name"`
		EID  string `json:"eid"`
	}
	if err := c.Do(context.Background(), "POST", "/api/hr-cli/v1/employee/get",
		map[string]string{"badge": "P000487"}, &out); err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if out.Name != "wubang" || out.EID != "94" {
		t.Fatalf("decoded payload mismatch: %+v", out)
	}
}

func TestDo_sendsConfirmHeader(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-HR-Confirm")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"data":null}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, &fakeTokens{access: "t"}, WithConfirm())
	if err := c.Do(context.Background(), "POST", "/api/hr-cli/v1/transfer/apply",
		map[string]string{"preview_id": "abc"}, nil); err != nil {
		t.Fatal(err)
	}
	if v := <-got; v != "yes" {
		t.Fatalf("X-HR-Confirm=%q want yes", v)
	}
}

func TestDo_envelopeErrorMappedToErrsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"ok":false,"error":{"type":"authorization","subtype":"target_out_of_scope","message":"eid 94 not in HRBP scope","hint":"contact admin"}}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, &fakeTokens{access: "t"})
	err := c.Do(context.Background(), "POST", "/x", nil, nil)
	if err == nil || err.Type != "authorization" || err.Subtype != "target_out_of_scope" {
		t.Fatalf("unexpected err: %v", err)
	}
	if err.Hint != "contact admin" {
		t.Fatalf("hint not propagated: %q", err.Hint)
	}
	if err.ExitCode != 3 {
		t.Fatalf("exit code for authorization should be 3, got %d", err.ExitCode)
	}
}

func TestDo_refreshesOnceOn401TokenExpired(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			if r.Header.Get("Authorization") != "Bearer initial" {
				t.Errorf("first call should carry initial token")
			}
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"ok":false,"error":{"type":"authentication","subtype":"token_expired","message":"jwt exp"}}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer refreshed" {
			t.Errorf("second call should carry refreshed token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"data":{"ok":1}}`)
	}))
	defer srv.Close()
	tokens := &fakeTokens{access: "initial", refreshed: "refreshed"}
	c, _ := New(srv.URL, tokens)
	var out map[string]int
	if err := c.Do(context.Background(), "GET", "/v1/auth/me", nil, &out); err != nil {
		t.Fatalf("Do() should succeed after refresh, got %v", err)
	}
	if atomic.LoadInt32(&tokens.refreshHits) != 1 {
		t.Fatalf("refresh should fire exactly once, got %d", tokens.refreshHits)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("server should be hit twice, got %d", calls)
	}
}

func TestDo_doesNotRetryOnNonExpiredAuthErrors(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":{"type":"authentication","subtype":"token_invalid","message":"bad sig"}}`)
	}))
	defer srv.Close()
	tokens := &fakeTokens{access: "t", refreshed: "r"}
	c, _ := New(srv.URL, tokens)
	err := c.Do(context.Background(), "GET", "/v1/auth/me", nil, nil)
	if err == nil || err.Subtype != "token_invalid" {
		t.Fatalf("expected token_invalid surfaced as-is, got %v", err)
	}
	if atomic.LoadInt32(&tokens.refreshHits) != 0 {
		t.Fatalf("non-expired auth errors must not trigger refresh, got %d hits", tokens.refreshHits)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("server should be hit exactly once, got %d", calls)
	}
}

func TestDo_emptyBodyHTTPErrorMappedToNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, &fakeTokens{access: "t"})
	err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if err == nil || err.Type != "network" || err.Subtype != "gateway_empty_response" {
		t.Fatalf("expected gateway_empty_response, got %v", err)
	}
}

func TestDo_malformedEnvelopeMappedToNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<html>upstream broken</html>`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, &fakeTokens{access: "t"})
	err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if err == nil || err.Subtype != "gateway_bad_envelope" {
		t.Fatalf("expected gateway_bad_envelope, got %v", err)
	}
}

func TestDo_sendsBodyAndUserAgent(t *testing.T) {
	type req struct {
		Badge string `json:"badge"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got req
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("body decode: %v", err)
		}
		if got.Badge != "P000487" {
			t.Errorf("badge=%q want P000487", got.Badge)
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "hr-cli/") {
			t.Errorf("User-Agent=%q want hr-cli/...", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"data":null}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, &fakeTokens{access: "t"})
	if err := c.Do(context.Background(), "POST", "/v1/employee/get", req{Badge: "P000487"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestParseError_unexpectedOK(t *testing.T) {
	err := parseError(500, []byte(`{"ok":true}`))
	if err == nil || err.Subtype != "gateway_unexpected_ok" {
		t.Fatalf("expected gateway_unexpected_ok, got %v", err)
	}
}

func TestParseError_emptyType(t *testing.T) {
	err := parseError(500, []byte(`{"ok":false,"error":{}}`))
	if err == nil || err.Subtype != "gateway_bad_envelope" {
		t.Fatalf("expected gateway_bad_envelope for empty type, got %v", err)
	}
}

func TestExitCodeFor(t *testing.T) {
	cases := map[string]int{
		"validation": 2, "config": 2,
		"authentication": 3, "authorization": 3, "policy": 3, "confirmation": 3,
		"db": 4, "network": 4,
		"internal": 5,
		"unknown":  1,
	}
	for typ, want := range cases {
		if got := exitCodeFor(typ); got != want {
			t.Errorf("exitCodeFor(%q)=%d want %d", typ, got, want)
		}
	}
}
