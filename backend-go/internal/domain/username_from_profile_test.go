package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUsernameFromProfile(t *testing.T) {
	cases := []struct {
		name  string
		pName string
		email string
		want  string
	}{
		{"表示名があれば、それを使う", "Alice Smith", "alice@example.com", "Alice Smith"},
		{"表示名の前後と途中の余分な空白は、1 つにまとめる", "  Alice \n  Smith ", "a@example.com", "Alice Smith"},
		{"表示名に制御文字があれば、空白として扱う", "Ali\x00ce", "a@example.com", "Ali ce"},
		{"表示名が空なら、メールの @ より前を使う", "", "alice.smith@example.com", "alice.smith"},
		{"表示名が空白だけでも、メールの @ より前を使う", "   ", "bob@example.com", "bob"},
		{"どちらからも材料が得られなければ、既定の名前を使う", "", "@example.com", usernameFallback},
		{"メールがなく、表示名も空なら、既定の名前を使う", "", "", usernameFallback},
		{"日本語の表示名は、そのまま使う", "山田 太郎", "taro@example.com", "山田 太郎"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := UsernameFromProfile(tt.pName, tt.email)
			if got != tt.want {
				t.Fatalf("UsernameFromProfile(%q, %q) = %q, want %q", tt.pName, tt.email, got, tt.want)
			}
			if msgs := ValidateUsername(got); len(msgs) > 0 {
				t.Fatalf("結果がユーザー名の規則を満たさない: %v", msgs)
			}
		})
	}

	t.Run("上限を超える長い表示名は、文字の途中で切らずに、上限の文字数までに切り詰める", func(t *testing.T) {
		for _, unit := range []string{"a", "あ", "🍔"} {
			got := UsernameFromProfile(strings.Repeat(unit, MaxUsernameChars+30), "a@example.com")
			if n := utf8.RuneCountInString(got); n != MaxUsernameChars {
				t.Fatalf("単位 %q: %d 文字, want %d", unit, n, MaxUsernameChars)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("単位 %q: 不正な UTF-8 になった", unit)
			}
			if msgs := ValidateUsername(got); len(msgs) > 0 {
				t.Fatalf("単位 %q: 規則を満たさない: %v", unit, msgs)
			}
		}
	})
}
