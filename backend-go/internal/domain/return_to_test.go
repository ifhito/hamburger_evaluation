package domain

import (
	"strings"
	"testing"
)

func TestSanitizeReturnTo(t *testing.T) {
	long := "/" + strings.Repeat("a", returnToMaxBytes)
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"アプリの中のパスは、そのまま使える", "/shops", "/shops"},
		{"クエリつきのパス(許可の画面への戻り)も、そのまま使える", "/oauth/authorize?client_id=x&redirect_uri=http%3A%2F%2Flocalhost%3A8788%2Fcallback", "/oauth/authorize?client_id=x&redirect_uri=http%3A%2F%2Flocalhost%3A8788%2Fcallback"},
		{"クエリの中に外部の URL が入っていても、先頭がアプリのパスなら使える", "/x?next=https://evil.example", "/x?next=https://evil.example"},
		{"空は、既定の画面を意味する空を返す", "", ""},
		{"外部の URL は、断る", "https://evil.example/", ""},
		{"スキームのない外部への移動(//host)は、断る", "//evil.example", ""},
		{"ブラウザが // と解釈する /\\host は、断る", `/\evil.example`, ""},
		{"バックスラッシュを含むものは、断る", `/a\b`, ""},
		{"javascript: は、断る", "javascript:alert(1)", ""},
		{"先頭が / でない相対のパスは、断る", "shops", ""},
		{"タブを含むものは(ブラウザが取り除いて // になりうるので)断る", "/\t/evil.example", ""},
		{"改行を含むものは、断る", "/a\nb", ""},
		{"削除文字を含むものは、断る", "/a\x7fb", ""},
		{"長すぎるものは、断る", long, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeReturnTo(tt.raw); got != tt.want {
				t.Fatalf("SanitizeReturnTo(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
