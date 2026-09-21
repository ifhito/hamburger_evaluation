package handler_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestServerErrorsStayEnglish は、5xx の本文が、Accept-Language によらず、英語の固定の文字列であることを
// 確かめる(詳細は、ログにだけ残す。利用者に見せる文言は、frontend が決める)。
func TestServerErrorsStayEnglish(t *testing.T) {
	t.Run("503 database unavailable", func(t *testing.T) {
		rec := doWithLanguage(newTestRouter(t, failPinger), http.MethodGet, "/up", "", "", "ja")
		if rec.Code != http.StatusServiceUnavailable || rec.Body.String() != `{"error":"database unavailable"}` {
			t.Errorf("status/body = %d %s, want 503 と英語の固定の文字列", rec.Code, rec.Body)
		}
		if got := rec.Header().Values("Vary"); len(got) != 0 {
			t.Errorf("Vary = %q, want なし(言語で変わらない)", got)
		}
	})

	t.Run("500 internal server error", func(t *testing.T) {
		kit := newSignupKit(t)
		kit.store.err = errors.New("boom")
		rec := doWithLanguage(kit.router, http.MethodPost, "/signup", signupBody("eve", "eve@example.com", "Password123!"), "", "ja")
		if rec.Code != http.StatusInternalServerError || rec.Body.String() != `{"error":"internal server error"}` {
			t.Errorf("status/body = %d %s, want 500 と英語の固定の文字列", rec.Code, rec.Body)
		}
	})
}

// TestMCPErrorsFollowAcceptLanguage は、/mcp の入り口の 4xx(Origin の拒否・トークンなし・無効なトークン)が、
// Accept-Language の言語で返ることを確かめる。ツールの中のエラーも同じ。
func TestMCPErrorsFollowAcceptLanguage(t *testing.T) {
	k := newMCPKit(t)
	languages := []struct {
		name, header, forbidden, unauthorized string
	}{
		{"指定なし", "", `{"error":"Forbidden"}`, `{"error":"Unauthorized"}`},
		{"ja", "ja", `{"error":"この操作をする権限がありません"}`, `{"error":"サインインが必要です"}`},
	}
	for _, l := range languages {
		headers := func(extra map[string]string) map[string]string {
			h := map[string]string{}
			if l.header != "" {
				h["Accept-Language"] = l.header
			}
			for k, v := range extra {
				h[k] = v
			}
			return h
		}
		t.Run("Origin の拒否 / "+l.name, func(t *testing.T) {
			resp, body := k.rpcWith(t, k.token(k.alice, readScope), rpcToolsList, headers(map[string]string{"Origin": "http://evil.example"}), "")
			if resp.StatusCode != http.StatusForbidden || body != l.forbidden {
				t.Errorf("status/body = %d %s, want 403 %s", resp.StatusCode, body, l.forbidden)
			}
		})
		t.Run("トークンなし / "+l.name, func(t *testing.T) {
			resp, body := k.rpcWith(t, "", rpcToolsList, headers(nil), "")
			if resp.StatusCode != http.StatusUnauthorized || body != l.unauthorized {
				t.Errorf("status/body = %d %s, want 401 %s", resp.StatusCode, body, l.unauthorized)
			}
		})
		t.Run("無効なトークン / "+l.name, func(t *testing.T) {
			resp, body := k.rpcWith(t, "not-a-real-token", rpcToolsList, headers(nil), "")
			if resp.StatusCode != http.StatusUnauthorized || body != l.unauthorized {
				t.Errorf("status/body = %d %s, want 401 %s", resp.StatusCode, body, l.unauthorized)
			}
		})
	}

	t.Run("ツールの中のエラーも、言語に従う(ヘッダーなしは英語)", func(t *testing.T) {
		for header, want := range map[string]string{"": "Review not found", "ja": "レビューが見つかりません"} {
			headers := map[string]string{}
			if header != "" {
				headers["Accept-Language"] = header
			}
			resp, body := k.rpcWith(t, k.token(k.alice, readScope), rpcToolCall("get_review", `{"review_id":"`+uid.N(999)+`"}`), headers, "")
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"isError":true`) || !strings.Contains(body, `"text":"`+want+`"`) {
				t.Errorf("Accept-Language %q: status/body = %d %s, want 200 の失敗で、文言 %q", header, resp.StatusCode, body, want)
			}
		}
	})

	t.Run("ツールの入力が検証で断られると、その文言も、要求の言語で返る(複数あれば「; 」でつなぐ)", func(t *testing.T) {
		args := `{"shop_id":"` + activeShopID + `","burger_id":"` + cheeseBurgerID + `","rating":0,"comment":""}`
		for header, want := range map[string]string{
			"":   "Rating must be in 1..5; Comment can't be blank",
			"ja": "評価は 1〜5 の整数で指定してください; コメントを入力してください",
		} {
			headers := map[string]string{}
			if header != "" {
				headers["Accept-Language"] = header
			}
			resp, body := k.rpcWith(t, k.token(k.alice, writeScope), rpcToolCall("create_review", args), headers, "")
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"isError":true`) || !strings.Contains(body, `"text":"`+want+`"`) {
				t.Errorf("Accept-Language %q: status/body = %d %s, want 200 の失敗で、文言 %q", header, resp.StatusCode, body, want)
			}
		}
	})

	t.Run("範囲が足りないときの 403 の本文も、言語に従う(WWW-Authenticate は、プロトコルなので英語のまま)", func(t *testing.T) {
		for header, want := range map[string]string{"": `{"error":"Insufficient scope: hamburger:write"}`, "ja": `{"error":"許可の範囲が足りません: hamburger:write"}`} {
			headers := map[string]string{}
			if header != "" {
				headers["Accept-Language"] = header
			}
			resp, body := k.rpcWith(t, k.token(k.alice, readScope), rpcToolCall("delete_review", `{}`), headers, "")
			if resp.StatusCode != http.StatusForbidden || body != want || !strings.Contains(resp.Header.Get("WWW-Authenticate"), `error="insufficient_scope"`) {
				t.Errorf("Accept-Language %q: status/body/challenge = %d %s %q, want 403 %s と英語の challenge", header, resp.StatusCode, body, resp.Header.Get("WWW-Authenticate"), want)
			}
		}
	})
}
