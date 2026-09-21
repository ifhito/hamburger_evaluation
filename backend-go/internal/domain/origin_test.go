package domain

import (
	"errors"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	t.Run("正規の Origin は、そのまま(scheme と host は小文字、既定のポートは省いた形)になる", func(t *testing.T) {
		for raw, want := range map[string]string{
			"http://localhost:8080":       "http://localhost:8080",
			"https://app.example.com":     "https://app.example.com",
			"HTTPS://App.Example.COM":     "https://app.example.com",
			"http://example.com:80":       "http://example.com",
			"https://example.com:443":     "https://example.com",
			"https://example.com:80":      "https://example.com:80",
			"http://example.com:443":      "http://example.com:443",
			"http://127.0.0.1:5173":       "http://127.0.0.1:5173",
			"http://[::1]:8080":           "http://[::1]:8080",
			"http://[::1]":                "http://[::1]",
			"http://[0:0:0:0:0:0:0:1]:80": "http://[0:0:0:0:0:0:0:1]",
			"https://xn--r8jz45g.example": "https://xn--r8jz45g.example",
		} {
			got, err := NormalizeOrigin(raw)
			if err != nil || got != want {
				t.Errorf("NormalizeOrigin(%q) = %q, %v, want %q", raw, got, err, want)
			}
		}
	})

	t.Run("使えない値は、理由に関わらず ErrInvalidOrigin になる(null・ワイルドカード・path・末尾のスラッシュ・query など)", func(t *testing.T) {
		for _, raw := range []string{
			"", "null", "NULL", "*", "example.com", "//example.com", "http://", "http:///path",
			"ftp://example.com", "file:///etc/passwd", "chrome-extension://abcdef", "javascript:alert(1)",
			"http://example.com/", "http://example.com/path", "http://example.com?x=1", "http://example.com#frag",
			"http://user@example.com", "http://user:pass@example.com", "http://example.com:", "http://example.com:0",
			"http://example.com:65536", "http://example.com:-1", "http://example.com:080", "http://example.com:http",
			"http://exa mple.com", " http://example.com", "http://example.com ", "http://example.com,http://evil.example",
		} {
			if got, err := NormalizeOrigin(raw); !errors.Is(err, ErrInvalidOrigin) {
				t.Errorf("NormalizeOrigin(%q) = %q, %v, want ErrInvalidOrigin", raw, got, err)
			}
		}
	})
}
