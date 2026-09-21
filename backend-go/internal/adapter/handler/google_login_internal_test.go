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

func testFlowCookie(t *testing.T, secret string) flowCookie {
	t.Helper()
	g, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://localhost:5173", RedirectURL: "http://localhost:8080/auth/google/callback", CookieSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	return g.cookie
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

	t.Run("封じた値は、期限内なら、元の手続きに戻り、中身は平文で見えない", func(t *testing.T) {
		entries := []flowEntry{testEntry("a", now), testEntry("b", now)}
		value, err := c.seal(entries)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"state-value-a", "nonce-value-a", "verifier-value-b", "/shops"} {
			if strings.Contains(value, secret) {
				t.Errorf("cookie の値に、平文の %q が含まれている", secret)
			}
		}
		got := c.open(value, now.Add(time.Minute))
		if len(got) != 2 || got[0] != entries[0] || got[1] != entries[1] {
			t.Fatalf("got = %+v", got)
		}
	})

	t.Run("同じ内容でも、封じるたびに、違う値になる", func(t *testing.T) {
		a, _ := c.seal([]flowEntry{testEntry("a", now)})
		b, _ := c.seal([]flowEntry{testEntry("a", now)})
		if a == b {
			t.Fatal("同じ値になった")
		}
	})

	t.Run("有効期間を過ぎた手続きは、開けない(期限内の手続きだけが残る)", func(t *testing.T) {
		old := testEntry("old", now.Add(-time.Hour))
		fresh := testEntry("fresh", now)
		value, _ := c.seal([]flowEntry{old, fresh})
		got := c.open(value, now)
		if len(got) != 1 || got[0].State != "state-value-fresh" {
			t.Fatalf("got = %+v, want 期限内の fresh だけ", got)
		}
	})

	t.Run("1 バイトでも書き換えられた値は、開けない", func(t *testing.T) {
		value, _ := c.seal([]flowEntry{testEntry("a", now)})
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		// 暗号化された中身の、どの 1 バイトを変えても(先頭の乱数・本文・認証の印のどこでも)、開けない。
		for i := range raw {
			mutated := append([]byte(nil), raw...)
			mutated[i] ^= 0x01
			if got := c.open(base64.RawURLEncoding.EncodeToString(mutated), now); len(got) != 0 {
				t.Fatalf("%d バイト目を書き換えた値を開けてしまった", i)
			}
		}
	})

	t.Run("別の秘密で封じた値・壊れた値・空の値は、開けない", func(t *testing.T) {
		other := testFlowCookie(t, "cookie-secret-B")
		foreign, _ := other.seal([]flowEntry{testEntry("a", now)})
		for name, v := range map[string]string{"別の秘密": foreign, "空": "", "短すぎる": "abc", "base64 でない": "!!!not-base64!!!"} {
			if got := c.open(v, now); len(got) != 0 {
				t.Errorf("%s の値を開けてしまった", name)
			}
		}
	})

	t.Run("cookie の暗号鍵は、秘密ごとに違い、空の秘密・不正な戻り先は受け付けない", func(t *testing.T) {
		if _, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://x", RedirectURL: "http://localhost:8080/cb", CookieSecret: ""}); err == nil {
			t.Fatal("空の秘密を受け付けた")
		}
		if _, err := NewGoogleLogin(nil, GoogleLoginConfig{AppBaseURL: "http://x", RedirectURL: "not a url", CookieSecret: "s"}); err == nil {
			t.Fatal("不正な戻り先を受け付けた")
		}
	})
}

// cookieOf は、応答に設定された、手続きの cookie を返す(なければ nil)。
func cookieOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == googleFlowCookieName {
			return c
		}
	}
	return nil
}

func requestWith(c *http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
	if c != nil {
		req.AddCookie(c)
	}
	return req
}

func flowOf(n string) usecase.GoogleFlow {
	return usecase.GoogleFlow{Secrets: usecase.GoogleFlowSecrets{State: "state-" + n, Nonce: "nonce-" + n, Verifier: "verifier-" + n}, ReturnTo: "/" + n}
}

func TestFlowCookieHoldsSeveralFlows(t *testing.T) {
	now := time.Now()
	c := testFlowCookie(t, "cookie-secret-A")

	t.Run("続けて手続きを始めると、先発の手続きは残り、state で取り出せる(取り出した手続きだけが cookie から消える)", func(t *testing.T) {
		rec1 := httptest.NewRecorder()
		if err := c.add(rec1, requestWith(nil), flowOf("one"), now); err != nil {
			t.Fatal(err)
		}
		rec2 := httptest.NewRecorder()
		if err := c.add(rec2, requestWith(cookieOf(rec1)), flowOf("two"), now); err != nil {
			t.Fatal(err)
		}
		jar := cookieOf(rec2)

		rec3 := httptest.NewRecorder()
		got, ok := c.take(rec3, requestWith(jar), "state-one", now)
		if !ok || got.ReturnTo != "/one" || got.Secrets.Nonce != "nonce-one" {
			t.Fatalf("先発の手続き = %+v, %v", got, ok)
		}
		rest := c.open(cookieOf(rec3).Value, now)
		if len(rest) != 1 || rest[0].State != "state-two" {
			t.Fatalf("残った手続き = %+v, want 後発だけ", rest)
		}

		rec4 := httptest.NewRecorder()
		if got, ok := c.take(rec4, requestWith(cookieOf(rec3)), "state-two", now); !ok || got.ReturnTo != "/two" {
			t.Fatalf("後発の手続き = %+v, %v", got, ok)
		}
		if final := cookieOf(rec4); final == nil || final.MaxAge >= 0 {
			t.Fatalf("手続きがなくなったら、cookie を消す: %+v", final)
		}
	})

	t.Run("知らない state・空の state では、何も取り出さず、cookie も変えない", func(t *testing.T) {
		rec := httptest.NewRecorder()
		_ = c.add(rec, requestWith(nil), flowOf("one"), now)
		jar := cookieOf(rec)
		for _, state := range []string{"", "state-unknown", "state-on"} {
			out := httptest.NewRecorder()
			if _, ok := c.take(out, requestWith(jar), state, now); ok {
				t.Errorf("state %q で取り出せてしまった", state)
			}
			if cookieOf(out) != nil {
				t.Errorf("state %q: cookie を変えた", state)
			}
		}
	})

	t.Run("同時に持てる手続きは上限まで(超えたら、いちばん古いものから捨てる)", func(t *testing.T) {
		var jar *http.Cookie
		for i := 0; i < googleFlowMaxFlows+2; i++ {
			rec := httptest.NewRecorder()
			if err := c.add(rec, requestWith(jar), flowOf(string(rune('a'+i))), now); err != nil {
				t.Fatal(err)
			}
			jar = cookieOf(rec)
		}
		entries := c.open(jar.Value, now)
		if len(entries) != googleFlowMaxFlows || entries[0].State != "state-c" || entries[len(entries)-1].State != "state-g" {
			t.Fatalf("残った手続き = %d 件(先頭 %q・末尾 %q), want 上限の %d 件で、古い a・b が捨てられている", len(entries), entries[0].State, entries[len(entries)-1].State, googleFlowMaxFlows)
		}
	})

	t.Run("期限切れの手続きは、新しく始めるときに掃除される", func(t *testing.T) {
		rec := httptest.NewRecorder()
		_ = c.add(rec, requestWith(nil), flowOf("stale"), now.Add(-2*googleFlowCookieTTL))
		rec2 := httptest.NewRecorder()
		if err := c.add(rec2, requestWith(cookieOf(rec)), flowOf("fresh"), now); err != nil {
			t.Fatal(err)
		}
		entries := c.open(cookieOf(rec2).Value, now)
		if len(entries) != 1 || entries[0].State != "state-fresh" {
			t.Fatalf("残った手続き = %+v, want fresh だけ", entries)
		}
	})

	t.Run("戻り先が長くても、封じた cookie の値は、大きさの上限を超えず、古い手続きから捨てられる(新しい手続きは残る)", func(t *testing.T) {
		long := "/oauth/authorize?" + strings.Repeat("a=b&", 500) // 2,000 バイト前後(& は、そのままの 1 バイト)
		if got := domain.SanitizeReturnTo(long); got != long {
			t.Fatalf("テストの戻り先が、規則を満たさない(%d バイト)", len(long))
		}
		var jar *http.Cookie
		for i := 0; i < googleFlowMaxFlows; i++ {
			f := flowOf(string(rune('a' + i)))
			f.ReturnTo = long
			rec := httptest.NewRecorder()
			if err := c.add(rec, requestWith(jar), f, now); err != nil {
				t.Fatal(err)
			}
			jar = cookieOf(rec)
			if len(jar.Value) > googleFlowCookieMaxValueBytes {
				t.Fatalf("cookie の値が %d バイト(上限 %d)", len(jar.Value), googleFlowCookieMaxValueBytes)
			}
		}
		entries := c.open(jar.Value, now)
		if len(entries) == 0 || entries[len(entries)-1].State != "state-e" {
			t.Fatalf("いちばん新しい手続きが残っていない: %+v", entries)
		}
	})

	t.Run("1 件だけでも大きすぎる手続きは、始められない(エラー)", func(t *testing.T) {
		f := flowOf("huge")
		f.ReturnTo = strings.Repeat("あ", 5000) // 実際には SanitizeReturnTo が長さで断るが、cookie の側でも上限を守る
		if err := c.add(httptest.NewRecorder(), requestWith(nil), f, now); err == nil {
			t.Fatal("大きすぎる手続きを、cookie に設定できてしまった")
		}
	})
}
