package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
)

// pingerFake is a hand-written fake for handler.Pinger; it returns err on
// every Ping.
type pingerFake struct{ err error }

func (p pingerFake) Ping(context.Context) error { return p.err }

var (
	okPinger   = pingerFake{}
	failPinger = pingerFake{err: errors.New("db down")}
)

// decodeError asserts body matches the {"error":"..."} shape and returns
// the message.
func decodeError(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", body, err)
	}
	if resp.Error == "" {
		t.Fatalf(`body %q does not match the {"error":"..."} shape`, body)
	}
	return resp.Error
}

// TestHealth covers AC1 (healthy pinger -> 200 {"status":"ok"}) and AC2
// (failing pinger -> 503 with the error shape).
func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		pinger     handler.Pinger
		wantStatus int
		wantBody   string // exact body; empty means assert error shape instead
	}{
		{
			name:       "AC1 healthy db returns 200 ok",
			pinger:     okPinger,
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "AC2 failing db returns 503 error shape",
			pinger:     failPinger,
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/up", nil)
			newTestRouter(tt.pinger).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if tt.wantBody != "" {
				if got := rec.Body.String(); got != tt.wantBody {
					t.Errorf("body = %q, want exactly %q", got, tt.wantBody)
				}
				return
			}
			decodeError(t, rec.Body.Bytes())
		})
	}
}

// TestUnknownRouteAndMethod covers the routing error shapes: unknown paths
// get 404 and wrong methods 405, both as {"error":"..."} JSON.
func TestUnknownRouteAndMethod(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string
	}{
		{name: "unknown route returns 404", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound},
		{name: "wrong method returns 405", method: http.MethodPost, path: "/up", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET /signup returns 405 Allow POST", method: http.MethodGet, path: "/signup", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			newTestRouter(okPinger).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			decodeError(t, rec.Body.Bytes())
			if tt.wantAllow != "" {
				if allow := rec.Header().Get("Allow"); allow != tt.wantAllow {
					t.Errorf("Allow = %q, want %q", allow, tt.wantAllow)
				}
			}
		})
	}
}

// TestBodyLimit covers AC3: a POST with a 2 MiB body gets 413 with the
// error JSON shape, and a subsequent request on the same client succeeds.
func TestBodyLimit(t *testing.T) {
	srv := httptest.NewServer(newTestRouter(okPinger))
	defer srv.Close()
	client := srv.Client()

	big := bytes.Repeat([]byte("a"), 2<<20) // 2 MiB
	resp, err := client.Post(srv.URL+"/up", "application/octet-stream", bytes.NewReader(big))
	if err != nil {
		t.Fatalf("oversized POST failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read 413 body: %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	decodeError(t, body)

	// The same client must still be able to talk to the server.
	resp2, err := client.Get(srv.URL + "/up")
	if err != nil {
		t.Fatalf("follow-up request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("follow-up status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}
}
