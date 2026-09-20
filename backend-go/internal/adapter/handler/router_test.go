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

// pingerFake は handler.Pinger の手書きの fake であり、Ping のたびに err を
// 返す。
type pingerFake struct{ err error }

func (p pingerFake) Ping(context.Context) error { return p.err }

var (
	okPinger   = pingerFake{}
	failPinger = pingerFake{err: errors.New("db down")}
)

// decodeError は、body が {"error":"..."} の形と一致することを検証し、その
// メッセージを返す。
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

// TestHealth は AC1（healthy な pinger -> 200 {"status":"ok"}）と AC2（失敗する
// pinger -> 503 とエラーの形）を扱う。
func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		pinger     handler.Pinger
		wantStatus int
		wantBody   string // 完全一致させる body。空なら代わりにエラーの形を検証する
	}{
		{
			name:       "AC1 DB が正常なら 200 ok を返す",
			pinger:     okPinger,
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "AC2 DB が失敗すると 503 とエラーの形を返す",
			pinger:     failPinger,
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/up", nil)
			newTestRouter(t, tt.pinger).ServeHTTP(rec, req)

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

// TestUnknownRouteAndMethod は routing のエラーの形を扱う：未知のパスは 404、
// 誤ったメソッドは 405 になり、どちらも {"error":"..."} の JSON で返る。
func TestUnknownRouteAndMethod(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string
	}{
		{name: "未知のルートは 404 を返す", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound},
		{name: "誤ったメソッドは 405 を返す", method: http.MethodPost, path: "/up", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET /signup は 405 を返し Allow は POST になる", method: http.MethodGet, path: "/signup", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
		{name: "DELETE /shops は 405 を返し Allow は GET, POST になる", method: http.MethodDelete, path: "/shops", wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, POST"},
		{name: "DELETE /shops/1 は 405 を返し Allow は GET になる", method: http.MethodDelete, path: "/shops/1", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "PATCH /reviews は 405 を返し Allow は GET, POST になる", method: http.MethodPatch, path: "/reviews", wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, POST"},
		{name: "PATCH /reviews/1 は 405 を返し Allow は DELETE, GET, PUT になる", method: http.MethodPatch, path: "/reviews/1", wantStatus: http.StatusMethodNotAllowed, wantAllow: "DELETE, GET, PUT"},
		{name: "POST /users は 405 を返し Allow は GET になる", method: http.MethodPost, path: "/users", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET /users/1 は 405 を返し Allow は DELETE, PUT になる", method: http.MethodGet, path: "/users/1", wantStatus: http.StatusMethodNotAllowed, wantAllow: "DELETE, PUT"},
		{name: "GET /admin/shops/1/approve は 405 を返し Allow は POST になる", method: http.MethodGet, path: "/admin/shops/1/approve", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			newTestRouter(t, okPinger).ServeHTTP(rec, req)

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

// TestBodyLimit は AC3 を扱う：2 MiB の body を持つ POST は、エラーの JSON の
// 形を伴う 413 になり、同じ client での後続の request は成功する。
func TestBodyLimit(t *testing.T) {
	srv := httptest.NewServer(newTestRouter(t, okPinger))
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

	// 同じ client が、引き続き server と通信できなければならない。
	resp2, err := client.Get(srv.URL + "/up")
	if err != nil {
		t.Fatalf("follow-up request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("follow-up status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}
}
