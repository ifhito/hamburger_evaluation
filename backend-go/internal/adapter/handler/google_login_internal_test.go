package handler

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

func testFlowCookieFor(t *testing.T, redirect, secret string) flowCookie {
	t.Helper()
	g, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://localhost:5173", RedirectURL: redirect, CookieSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	return g.cookie
}

func testFlowCookie(t *testing.T, secret string) flowCookie {
	t.Helper()
	return testFlowCookieFor(t, "http://localhost:8080/api/auth/google/callback", secret)
}

func testEntry(n string, now time.Time) flowEntry {
	return flowEntry{
		State: "state-value-" + n, Nonce: "nonce-value-" + n, Verifier: "verifier-value-" + n,
		ReturnTo: "/shops", LinkUserID: "3f6c1a7e-8b21-4d5f-9a3e-2c7d41b0e9aa", ExpiresAt: now.Add(googleFlowCookieTTL).Unix(),
	}
}

func TestFlowCookieSealAndOpen(t *testing.T) {
	now := time.Now()
	c := testFlowCookie(t, "cookie-secret-A")
	const name = "google_login_flow_x"

	t.Run("封じた値は、期限内なら、元の手続きに戻り、中身は平文で見えない", func(t *testing.T) {
		entry := testEntry("a", now)
		value, err := c.seal(name, entry)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"state-value-a", "nonce-value-a", "verifier-value-a", "/shops"} {
			if strings.Contains(value, secret) {
				t.Errorf("cookie の値に、平文の %q が含まれている", secret)
			}
		}
		got, ok := c.open(name, value, now.Add(time.Minute))
		if !ok || got != entry {
			t.Fatalf("got = %+v, %v", got, ok)
		}
	})

	t.Run("同じ内容でも、封じるたびに、違う値になる", func(t *testing.T) {
		a, _ := c.seal(name, testEntry("a", now))
		b, _ := c.seal(name, testEntry("a", now))
		if a == b {
			t.Fatal("同じ値になった")
		}
	})

	t.Run("有効期間を過ぎた手続きは、開けない", func(t *testing.T) {
		value, _ := c.seal(name, testEntry("old", now.Add(-time.Hour)))
		if _, ok := c.open(name, value, now); ok {
			t.Fatal("期限切れの手続きを開けてしまった")
		}
	})

	t.Run("1 バイトでも書き換えられた値は、開けない", func(t *testing.T) {
		value, _ := c.seal(name, testEntry("a", now))
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		// 暗号化された中身の、どの 1 バイトを変えても(先頭の乱数・本文・認証の印のどこでも)、開けない。
		for i := range raw {
			mutated := append([]byte(nil), raw...)
			mutated[i] ^= 0x01
			if _, ok := c.open(name, base64.RawURLEncoding.EncodeToString(mutated), now); ok {
				t.Fatalf("%d バイト目を書き換えた値を開けてしまった", i)
			}
		}
	})

	t.Run("別の名前の cookie に移し替えた値は、開けない(名前も認証に含めている)", func(t *testing.T) {
		value, _ := c.seal("google_login_flow_A", testEntry("a", now))
		if _, ok := c.open("google_login_flow_B", value, now); ok {
			t.Fatal("別の名前の cookie の値として、開けてしまった")
		}
	})

	t.Run("別の秘密で封じた値・壊れた値・空の値は、開けない", func(t *testing.T) {
		other := testFlowCookie(t, "cookie-secret-B")
		foreign, _ := other.seal(name, testEntry("a", now))
		for label, v := range map[string]string{"別の秘密": foreign, "空": "", "短すぎる": "abc", "base64 でない": "!!!not-base64!!!"} {
			if _, ok := c.open(name, v, now); ok {
				t.Errorf("%s の値を開けてしまった", label)
			}
		}
	})
}

func TestNewGoogleLoginRejectsInvalidConfig(t *testing.T) {
	cases := map[string]GoogleLoginConfig{
		"空の秘密":       {AppBaseURL: "http://x", RedirectURL: "http://localhost:8080/auth/google/callback", CookieSecret: ""},
		"URL でない戻り先": {AppBaseURL: "http://x", RedirectURL: "not a url", CookieSecret: "s"},
		"path が /auth/google/callback で終わらない": {AppBaseURL: "http://x", RedirectURL: "http://localhost:8080/cb", CookieSecret: "s"},
		"path の末尾に / がある":                     {AppBaseURL: "http://x", RedirectURL: "http://localhost:8080/auth/google/callback/", CookieSecret: "s"},
		"path が空(根)":                          {AppBaseURL: "http://x", RedirectURL: "http://localhost:8080", CookieSecret: "s"},
	}
	for name, cfg := range cases {
		if _, err := NewGoogleLogin(nil, cfg); err == nil {
			t.Errorf("%s を受け付けた", name)
		}
	}
}

func TestFlowCookiePaths(t *testing.T) {
	cases := map[string]struct{ callback, exchange string }{
		"http://localhost:8080/api/auth/google/callback": {"/api/auth/google/callback", "/api/auth/google/exchange"},
		"http://localhost:8080/auth/google/callback":     {"/auth/google/callback", "/auth/google/exchange"},
		"https://x.example/v1/api/auth/google/callback":  {"/v1/api/auth/google/callback", "/v1/api/auth/google/exchange"},
	}
	for redirect, want := range cases {
		c := testFlowCookieFor(t, redirect, "s")
		if c.callbackPath != want.callback || c.exchangePath != want.exchange {
			t.Errorf("%s: callback = %q, exchange = %q, want %q・%q", redirect, c.callbackPath, c.exchangePath, want.callback, want.exchange)
		}
	}
}

func flowOf(n string) usecase.GoogleFlow {
	return usecase.GoogleFlow{Secrets: usecase.GoogleFlowSecrets{State: "state-" + n, Nonce: "nonce-" + n, Verifier: "verifier-" + n}, ReturnTo: "/" + n}
}

func requestWith(cookies ...*http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	return req
}

func TestFlowCookieSetAndTake(t *testing.T) {
	now := time.Now()
	c := testFlowCookie(t, "cookie-secret-A")

	set := func(t *testing.T, n string) *http.Cookie {
		t.Helper()
		rec := httptest.NewRecorder()
		if err := c.set(rec, flowOf(n), now); err != nil {
			t.Fatal(err)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("設定された cookie = %d 件, want 1", len(cookies))
		}
		return cookies[0]
	}

	t.Run("手続きごとに、別の名前の cookie が 1 つ設定され、Path は戻りの要求だけである", func(t *testing.T) {
		one, two := set(t, "one"), set(t, "two")
		if one.Name == two.Name || !strings.HasPrefix(one.Name, googleFlowCookiePrefix) {
			t.Fatalf("名前 = %q・%q, want 手続きごとに違う、%s で始まる名前", one.Name, two.Name, googleFlowCookiePrefix)
		}
		if strings.Contains(one.Name, "state-one") {
			t.Error("cookie の名前に、平文の state が含まれている")
		}
		if one.Path != "/api/auth/google/callback" || !one.HttpOnly || one.SameSite != http.SameSiteLaxMode || one.MaxAge != 600 || one.Secure {
			t.Fatalf("cookie = %+v", one)
		}
		secure := testFlowCookieFor(t, "https://api.example.com/auth/google/callback", "s")
		rec := httptest.NewRecorder()
		_ = secure.set(rec, flowOf("one"), now)
		if got := rec.Result().Cookies()[0]; !got.Secure || got.Path != "/auth/google/callback" {
			t.Fatalf("https の戻り先の cookie = %+v, want Secure・戻り先の path", got)
		}
	})

	t.Run("state で取り出せ、使った手続きの cookie だけが、同じ名前・Path で消える(ほかのタブの手続きは触れない)", func(t *testing.T) {
		one, two := set(t, "one"), set(t, "two")

		out := httptest.NewRecorder()
		got, ok := c.take(out, requestWith(one, two), "state-one", now)

		if !ok || got.ReturnTo != "/one" || got.Secrets.Nonce != "nonce-one" || got.Secrets.Verifier != "verifier-one" {
			t.Fatalf("先発の手続き = %+v, %v", got, ok)
		}
		cleared := out.Result().Cookies()
		if len(cleared) != 1 || cleared[0].Name != one.Name || cleared[0].Path != one.Path || cleared[0].MaxAge >= 0 {
			t.Fatalf("消した cookie = %+v, want 使った手続きの cookie(同じ名前・Path)だけ", cleared)
		}
		if _, ok := c.take(httptest.NewRecorder(), requestWith(two), "state-two", now); !ok {
			t.Fatal("後発の手続きを取り出せない")
		}
	})

	t.Run("知らない state・空の state・cookie がない要求では、何も取り出さず、何も消さない", func(t *testing.T) {
		one := set(t, "one")
		for _, state := range []string{"", "state-unknown", "state-on"} {
			out := httptest.NewRecorder()
			if _, ok := c.take(out, requestWith(one), state, now); ok {
				t.Errorf("state %q で取り出せてしまった", state)
			}
			if len(out.Result().Cookies()) != 0 {
				t.Errorf("state %q: cookie を変えた", state)
			}
		}
	})

	t.Run("改ざんされた・別の state の cookie の値・期限切れの手続きは、取り出せず、その cookie は消す", func(t *testing.T) {
		one, two := set(t, "one"), set(t, "two")
		tampered := *one
		tampered.Value = one.Value[:len(one.Value)-2] + "AA"
		moved := *one // state-two の名前の cookie に、state-one の値を移し替えたもの
		moved.Name = two.Name
		expired := httptest.NewRecorder()
		_ = c.set(expired, flowOf("old"), now.Add(-2*googleFlowCookieTTL))
		expiredCookie := expired.Result().Cookies()[0]

		for label, tc := range map[string]struct {
			cookie *http.Cookie
			state  string
		}{"改ざん": {&tampered, "state-one"}, "名前の移し替え": {&moved, "state-two"}, "期限切れ": {expiredCookie, "state-old"}} {
			out := httptest.NewRecorder()
			if _, ok := c.take(out, requestWith(tc.cookie), tc.state, now); ok {
				t.Errorf("%s: 取り出せてしまった", label)
			}
			if cleared := out.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge >= 0 {
				t.Errorf("%s: 使えない cookie を消していない: %+v", label, cleared)
			}
		}
	})
}

func TestBinderCookie(t *testing.T) {
	c := testFlowCookie(t, "cookie-secret-A")

	t.Run("結び付けの値の cookie は、コードごとに別の名前で、結果との交換の要求にだけ送られ、コードと同じ時間で切れる", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c.setBinder(rec, "code-one", "binder-one")
		c.setBinder(rec, "code-two", "binder-two")
		cookies := rec.Result().Cookies()
		if len(cookies) != 2 || cookies[0].Name == cookies[1].Name {
			t.Fatalf("cookie = %+v, want コードごとに別の名前の 2 件", cookies)
		}
		one := cookies[0]
		if !strings.HasPrefix(one.Name, googleHandoffCookiePrefix) || strings.Contains(one.Name, "code-one") {
			t.Errorf("名前 = %q", one.Name)
		}
		if one.Path != "/api/auth/google/exchange" || !one.HttpOnly || one.SameSite != http.SameSiteLaxMode || one.MaxAge != int(domain.LoginHandoffTTL.Seconds()) || one.Secure {
			t.Fatalf("cookie = %+v", one)
		}
	})

	t.Run("コードに対応する cookie の値だけを返す(ない・別のコードのものは空)", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c.setBinder(rec, "code-one", "binder-one")
		c.setBinder(rec, "code-two", "binder-two")
		req := requestWith(rec.Result().Cookies()...)
		if got := c.binder(req, "code-one"); got != "binder-one" {
			t.Errorf("code-one = %q", got)
		}
		if got := c.binder(req, "code-two"); got != "binder-two" {
			t.Errorf("code-two = %q", got)
		}
		for _, code := range []string{"code-three", ""} {
			if got := c.binder(req, code); got != "" {
				t.Errorf("code %q = %q, want 空", code, got)
			}
		}
	})

	t.Run("使い終わったら、同じ名前・Path で消す", func(t *testing.T) {
		set := httptest.NewRecorder()
		c.setBinder(set, "code-one", "binder-one")
		clear := httptest.NewRecorder()
		c.clearBinder(clear, "code-one")
		a, b := set.Result().Cookies()[0], clear.Result().Cookies()[0]
		if a.Name != b.Name || a.Path != b.Path || b.MaxAge >= 0 {
			t.Fatalf("設定 %+v・消去 %+v, want 同じ名前・Path で MaxAge < 0", a, b)
		}
	})
}
