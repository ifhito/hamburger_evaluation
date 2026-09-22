package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/fakeoidc"
)

// TestGoogleLoginMessagesFollowAcceptLanguage は、Google のサインインの案内(画面がそのまま出す文言)が、
// 交換の要求の Accept-Language の言語で返ることを、案内の種類ごとに確かめる。ヘッダーなしは、いまと同じ英語である。
func TestGoogleLoginMessagesFollowAcceptLanguage(t *testing.T) {
	type outcome struct {
		name   string
		status int
		en, ja string
		// setup は、kit ごとに 1 回、前提を整える(なくてもよい)。attempt は、その案内になる手続きを 1 回進め、
		// そのコードを返す(何回でも呼べる)。
		setup   func(t *testing.T, k *googleKit)
		attempt func(t *testing.T, k *googleKit) string
	}
	linkBob := func(t *testing.T, k *googleKit) {
		p := k.authorizeLink(t, k.bob, `{"return_to":"/profile"}`)
		if body := decodeExchange(t, k.exchange(k.callback(t, p, p.cookies, plainRun()).code)); !body.Linked {
			t.Fatal("bob の結び付けに失敗した")
		}
	}
	linkAs := func(t *testing.T, k *googleKit, userID string) string {
		p := k.authorizeLink(t, userID, `{"return_to":"/profile"}`)
		return k.callback(t, p, p.cookies, plainRun()).code
	}
	outcomes := []outcome{
		{
			name: "同じメールのアカウントがあるとき", status: http.StatusConflict,
			en: "An account with this email address already exists. Sign in with your password, then connect Google from your profile.",
			ja: "このメールアドレスのアカウントが、すでにあります。パスワードでサインインして、プロフィールから Google を連携してください。",
			attempt: func(t *testing.T, k *googleKit) string {
				k.idp.SetUser(fakeoidc.User{Sub: "sub-attacker", Email: "alice@example.com", EmailVerified: true, Name: "Alice Impostor"})
				return k.run(t, "").code
			},
		},
		{
			name: "別の利用者に連携済みの Google アカウントを結び付けようとしたとき", status: http.StatusConflict,
			en:    "This Google account is already connected to another account.",
			ja:    "この Google アカウントは、ほかのアカウントに連携済みです。",
			setup: linkBob,
			attempt: func(t *testing.T, k *googleKit) string {
				return linkAs(t, k, k.alice)
			},
		},
		{
			name: "すでに Google に連携している利用者が、別の Google を結び付けようとしたとき", status: http.StatusConflict,
			en:    "Your account is already connected to a Google account. Disconnect it first.",
			ja:    "このアカウントは、すでに Google アカウントに連携しています。先に、連携を解除してください。",
			setup: linkBob,
			attempt: func(t *testing.T, k *googleKit) string {
				k.idp.SetUser(fakeoidc.User{Sub: "sub-other", Email: "other@gmail.example", EmailVerified: true, Name: "Other"})
				return linkAs(t, k, k.bob)
			},
		},
		{
			name: "手続きが失敗したとき", status: http.StatusBadRequest,
			en: "Google sign-in failed. Please try again.",
			ja: "Google でのサインインに失敗しました。もう一度お試しください。",
			attempt: func(t *testing.T, k *googleKit) string {
				return k.run(t, "", withQuery(func(q url.Values) { q.Del("code") })).code
			},
		},
	}
	for _, o := range outcomes {
		t.Run(fmt.Sprintf("%s、%d で、言語に合わせた案内を返す", o.name, o.status), func(t *testing.T) {
			k := newGoogleKit(t)
			if o.setup != nil {
				o.setup(t, k)
			}
			for _, l := range []struct{ header, want string }{{"", o.en}, {"ja", o.ja}, {"ja-JP,ja;q=0.9,en;q=0.8", o.ja}, {"fr-FR", o.en}} {
				rec := k.exchangeIn(o.attempt(t, k), l.header)
				body := decodeExchange(t, rec)
				if rec.Code != o.status || len(body.Errors) != 1 || body.Errors[0] != l.want {
					t.Errorf("Accept-Language %q: status/errors = %d %s, want %d と %q", l.header, rec.Code, rec.Body, o.status, l.want)
				}
				if !slices.Contains(rec.Header().Values("Vary"), "Accept-Language") {
					t.Errorf("Accept-Language %q: Vary = %q", l.header, rec.Header().Values("Vary"))
				}
			}
		})
	}

	t.Run("存在しないコードを交換すると、400 で、言語に合わせた案内と、言語によらない reason を返す", func(t *testing.T) {
		k := newGoogleKit(t)
		for header, want := range map[string]string{
			"":   `{"errors":["The Google sign-in link is invalid or has expired. Please try again."],"return_to":"","reason":"google.code_invalid"}`,
			"ja": `{"errors":["Google のサインインのリンクが、無効か期限切れです。もう一度お試しください。"],"return_to":"","reason":"google.code_invalid"}`,
		} {
			rec := k.exchangeIn("no-such-code", header)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != want {
				t.Errorf("Accept-Language %q: %d %s, want 400 %s", header, rec.Code, rec.Body, want)
			}
		}
	})

	t.Run("パスワードのない利用者が Google の連携を解除しようとすると、422 で、言語に合わせた案内を返す", func(t *testing.T) {
		k := newGoogleKit(t)
		auth := "Bearer " + decodeExchange(t, k.exchange(k.run(t, "").code)).Token
		for header, want := range map[string]string{
			"":   `{"errors":["Google is your only way to sign in. Add a password before disconnecting it."]}`,
			"ja": `{"errors":["Google が、サインインできる唯一の方法です。パスワードを追加してから、連携を解除してください。"]}`,
		} {
			rec := doWithLanguage(k.router, http.MethodDelete, "/me/identities/google", "", auth, header)
			if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != want {
				t.Errorf("Accept-Language %q: %d %s, want 422 %s", header, rec.Code, rec.Body, want)
			}
		}
	})
}

// TestOAuthConsentMessagesFollowAcceptLanguage は、許可の画面の API(GET /oauth/authorize/request・
// POST /oauth/authorize/decision)の 422 が、Accept-Language の言語で返ることを確かめる。要求が不正な理由の
// 診断の文(英語)は、英語の応答では、いままでと同じ文字列そのままで、日本語の応答では、日本語の説明に添える。
func TestOAuthConsentMessagesFollowAcceptLanguage(t *testing.T) {
	_, challenge := pkce()
	k := newOAuthKit(t)

	t.Run("許可するかどうかが書かれていない決定を送ると、422 で、言語に合わせた案内を返す", func(t *testing.T) {
		for header, want := range map[string]string{
			"":   `{"errors":["Approve is required"]}`,
			"ja": `{"errors":["許可するか、許可しないかを指定してください"]}`,
		} {
			rec := doWithLanguage(k.router, http.MethodPost, "/oauth/authorize/decision", `{"query":"`+authQuery(challenge)+`"}`, k.bearer[k.alice], header)
			if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != want {
				t.Errorf("Accept-Language %q: %d %s, want 422 %s", header, rec.Code, rec.Body, want)
			}
		}
	})

	invalid := map[string]string{
		"登録されていない戻り先の要求":   authQuery(challenge, "redirect_uri", "https://evil.example.com/callback"),
		"知らないアプリの要求":       authQuery(challenge, "client_id", "unknown-app"),
		"PKCE がない":         authQuery(challenge, "code_challenge", "", "code_challenge_method", ""),
		"知らない範囲を求める要求":     authQuery(challenge, "scope", "admin"),
		"解釈できない query の要求": "a=%zz",
	}
	for name, query := range invalid {
		t.Run(name+"を送ると、422 で、言語に合わせた理由が返る", func(t *testing.T) {
			// 英語の応答(ヘッダーなし)は、いままでと同じ、診断の文字列そのまま。日本語は、その文字列を添える。
			for _, call := range []struct {
				name string
				do   func(header string) *httptest.ResponseRecorder
			}{
				{"describe", func(h string) *httptest.ResponseRecorder {
					return doWithLanguage(k.router, http.MethodGet, "/oauth/authorize/request?"+query, "", k.bearer[k.alice], h)
				}},
				{"decide", func(h string) *httptest.ResponseRecorder {
					body, _ := json.Marshal(map[string]any{"query": query, "approve": true})
					return doWithLanguage(k.router, http.MethodPost, "/oauth/authorize/decision", string(body), k.bearer[k.alice], h)
				}},
			} {
				var en struct {
					Error string `json:"error"`
				}
				rec := call.do("")
				if err := json.Unmarshal(rec.Body.Bytes(), &en); err != nil || rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("%s 英語 = %d %s", call.name, rec.Code, rec.Body)
				}
				var lead string
				switch {
				case strings.HasPrefix(en.Error, "oauth authorization request is invalid"):
					lead = "このアプリからの許可の要求が正しくありません。詳細: "
				case strings.HasPrefix(en.Error, "oauth scope is invalid"):
					lead = "要求された許可の範囲が正しくありません。詳細: "
				default:
					t.Fatalf("%s 英語の診断が、いままでの文字列で始まらない: %q", call.name, en.Error)
				}
				wantJA, _ := json.Marshal(map[string]string{"error": lead + en.Error})
				if ja := call.do("ja"); ja.Code != http.StatusUnprocessableEntity || ja.Body.String() != string(wantJA) {
					t.Errorf("%s ja: %d %s, want 422 %s", call.name, ja.Code, ja.Body, wantJA)
				}
			}
		})
	}
}
