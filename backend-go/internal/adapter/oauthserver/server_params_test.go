package oauthserver

import (
	"net/url"
	"testing"
)

// 既定の範囲(読み取りだけ)を補うのは、認可の要求だけである。トークンの要求(認可コードの交換・更新)で
// scope を省くのは「元の許可の範囲を保つ」という意味なので、既定の範囲で置き換えない。
func TestNormalizeParamsFillsTheDefaultScopeOnlyForAuthorizationRequests(t *testing.T) {
	s := &Server{cfg: Config{Resource: "https://api.example.com/mcp"}}

	t.Run("認可の要求で scope を省くと、既定の範囲(読み取りだけ)になる", func(t *testing.T) {
		q, err := s.normalizeParams(url.Values{}, true)
		if err != nil || q.Get("scope") != "hamburger:read" {
			t.Errorf("scope = %q, err = %v, want hamburger:read", q.Get("scope"), err)
		}
	})

	t.Run("トークンの要求で scope を省いても、既定の範囲を補わない", func(t *testing.T) {
		q, err := s.normalizeParams(url.Values{"grant_type": {"refresh_token"}}, false)
		if err != nil || q.Has("scope") {
			t.Errorf("scope = %q (present %v), err = %v, want no scope", q.Get("scope"), q.Has("scope"), err)
		}
	})

	t.Run("scope を指定した要求は、認可でもトークンでも、そのまま残る", func(t *testing.T) {
		for _, authorize := range []bool{true, false} {
			q, _ := s.normalizeParams(url.Values{"scope": {"hamburger:read hamburger:write"}}, authorize)
			if q.Get("scope") != "hamburger:read hamburger:write" {
				t.Errorf("authorize=%v: scope = %q", authorize, q.Get("scope"))
			}
		}
	})

	t.Run("宛先は、どちらの要求でも、この認可サーバーの宛先に固定される", func(t *testing.T) {
		for _, authorize := range []bool{true, false} {
			q, err := s.normalizeParams(url.Values{"audience": {"https://evil.example.com"}}, authorize)
			if err != nil || q.Get("audience") != "https://api.example.com/mcp" {
				t.Errorf("authorize=%v: audience = %q, err = %v", authorize, q.Get("audience"), err)
			}
		}
	})
}
