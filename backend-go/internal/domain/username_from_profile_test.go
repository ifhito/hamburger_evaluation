package domain

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// generatedUsername は、使える表示名がないときに作る、中立なユーザー名の形である。
var generatedUsername = regexp.MustCompile(`^user-[a-z2-7]{8}$`)

func TestUsernameFromProfile(t *testing.T) {
	cases := []struct {
		name  string
		pName string
		want  string
	}{
		{"表示名があれば、それを使う", "Alice Smith", "Alice Smith"},
		{"表示名の前後と途中の余分な空白は、1 つにまとめる", "  Alice \n  Smith ", "Alice Smith"},
		{"表示名に制御文字があれば、空白として扱う", "Ali\x00ce", "Ali ce"},
		{"日本語の表示名は、そのまま使う", "山田 太郎", "山田 太郎"},
		{"表示名の、双方向の上書き・埋め込み・分離の文字(Cf)は、取り除く(表示を逆転させて別人に見せかけられないように)", "\u202eAlice\u202c \u2066Smith\u2069", "Alice Smith"},
		{"表示名の、幅のない文字(ゼロ幅の空白・結合しない接合子など。Cf)は、取り除く", "Ca\u200brol\u200d\ufeff", "Carol"},
		{"表示名の、行・段落の区切り(Zl・Zp)と、全角の空白は、空白として扱い、1 つにまとめる", "Alice\u2028\u2029\u3000Smith", "Alice Smith"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := UsernameFromProfile(tt.pName)
			if got != tt.want {
				t.Fatalf("UsernameFromProfile(%q) = %q, want %q", tt.pName, got, tt.want)
			}
			if msgs := ValidateUsername(got); len(msgs) > 0 {
				t.Fatalf("結果がユーザー名の規則を満たさない: %v", msgs)
			}
		})
	}

	t.Run("使える表示名がなければ、メールとは無関係な、中立の名前を作る(材料は表示名だけで、メールは受け取らない)", func(t *testing.T) {
		for name, pName := range map[string]string{"空": "", "空白だけ": "   ", "取り除く文字だけ": "\u202e\u200b", "制御文字だけ": "\x00\x01"} {
			got := UsernameFromProfile(pName)
			if !generatedUsername.MatchString(got) {
				t.Fatalf("%s: UsernameFromProfile(%q) = %q, want user-<乱数 8 文字>", name, pName, got)
			}
			if msgs := ValidateUsername(got); len(msgs) > 0 {
				t.Fatalf("%s: 規則を満たさない: %v", name, msgs)
			}
		}
		if a, b := UsernameFromProfile(""), UsernameFromProfile(""); a == b {
			t.Fatalf("2 回とも同じ名前 %q(乱数になっていない)", a)
		}
	})

	t.Run("切り詰めた末尾が空白になるときは、その空白も取り除く(表示名の最後に空白が残らない)", func(t *testing.T) {
		name := strings.Repeat("a", MaxUsernameChars-1) + " b"
		got := UsernameFromProfile(name)
		if got != strings.Repeat("a", MaxUsernameChars-1) {
			t.Fatalf("UsernameFromProfile = %q, want 末尾の空白なしの %d 文字", got, MaxUsernameChars-1)
		}
	})

	t.Run("上限を超える長い表示名は、文字の途中で切らずに、上限の文字数までに切り詰める", func(t *testing.T) {
		for _, unit := range []string{"a", "あ", "🍔"} {
			got := UsernameFromProfile(strings.Repeat(unit, MaxUsernameChars+30))
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
