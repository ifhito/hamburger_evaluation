package handler

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

func testFlowCookie(t *testing.T, secret string) flowCookie {
	t.Helper()
	g, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://localhost:5173", RedirectURL: "http://localhost:8080/auth/google/callback", CookieSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	return g.cookie
}

func testFlow() usecase.GoogleFlow {
	return usecase.GoogleFlow{
		Secrets:    usecase.GoogleFlowSecrets{State: "state-value-abc", Nonce: "nonce-value-def", Verifier: "verifier-value-ghi"},
		ReturnTo:   "/shops",
		LinkUserID: "3f6c1a7e-8b21-4d5f-9a3e-2c7d41b0e9aa",
	}
}

func TestFlowCookieSealAndOpen(t *testing.T) {
	now := time.Now()
	c := testFlowCookie(t, "cookie-secret-A")

	t.Run("封じた値は、期限内なら、元の手続きの値に戻り、中身は平文で見えない", func(t *testing.T) {
		value, err := c.seal(testFlow(), now)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"state-value-abc", "nonce-value-def", "verifier-value-ghi", "/shops"} {
			if strings.Contains(value, secret) {
				t.Errorf("cookie の値に、平文の %q が含まれている", secret)
			}
		}
		got, ok := c.open(value, now.Add(time.Minute))
		if !ok || got != testFlow() {
			t.Fatalf("got = %+v, ok = %v", got, ok)
		}
	})

	t.Run("同じ内容でも、封じるたびに、違う値になる", func(t *testing.T) {
		a, _ := c.seal(testFlow(), now)
		b, _ := c.seal(testFlow(), now)
		if a == b {
			t.Fatal("同じ値になった")
		}
	})

	t.Run("有効期間を過ぎた値は、開けない", func(t *testing.T) {
		value, _ := c.seal(testFlow(), now)
		if _, ok := c.open(value, now.Add(googleFlowCookieTTL+time.Minute)); ok {
			t.Fatal("期限切れの cookie を開けてしまった")
		}
	})

	t.Run("1 バイトでも書き換えられた値は、開けない", func(t *testing.T) {
		value, _ := c.seal(testFlow(), now)
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		// 暗号化された中身の、どの 1 バイトを変えても(先頭の乱数・本文・認証の印のどこでも)、開けない。
		for i := range raw {
			mutated := append([]byte(nil), raw...)
			mutated[i] ^= 0x01
			if _, ok := c.open(base64.RawURLEncoding.EncodeToString(mutated), now); ok {
				t.Fatalf("%d バイト目を書き換えた値を開けてしまった", i)
			}
		}
	})

	t.Run("別の秘密で封じた値・壊れた値・空の値は、開けない", func(t *testing.T) {
		other := testFlowCookie(t, "cookie-secret-B")
		foreign, _ := other.seal(testFlow(), now)
		for name, v := range map[string]string{"別の秘密": foreign, "空": "", "短すぎる": "abc", "base64 でない": "!!!not-base64!!!"} {
			if _, ok := c.open(v, now); ok {
				t.Errorf("%s の値を開けてしまった", name)
			}
		}
	})

	t.Run("cookie の暗号鍵は、秘密ごとに違い、空の秘密は受け付けない", func(t *testing.T) {
		if _, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://x", RedirectURL: "http://localhost:8080/cb", CookieSecret: ""}); err == nil {
			t.Fatal("空の秘密を受け付けた")
		}
		if _, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://x", RedirectURL: "not a url", CookieSecret: "s"}); err == nil {
			t.Fatal("不正な戻り先を受け付けた")
		}
	})
}
