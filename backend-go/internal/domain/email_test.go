package domain

import (
	"reflect"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	const (
		blank   = "Email can't be blank"
		invalid = "Email is invalid"
	)
	tests := []struct {
		name  string
		email string
		want  []string
	}{
		{"空は blank だけを返す", "", []string{blank}},
		{"空白だけは invalid", " ", []string{invalid}},
		{"@ がなければ invalid", "abc", []string{invalid}},
		{"ローカル部だけは invalid", "a@", []string{invalid}},
		{"ドメイン部だけは invalid", "@b.c", []string{invalid}},
		{"@ が複数なら invalid", "a@b@c.d", []string{invalid}},
		{"空白を含むローカル部は invalid", "a b@c.d", []string{invalid}},
		{"前後の空白は invalid(保存する値を黙って加工しない)", " a@b.c ", []string{invalid}},
		{"表示名つきは invalid", "Name <a@b.c>", []string{invalid}},
		{"複数のアドレスは invalid", "a@b.c, d@e.f", []string{invalid}},
		{"コメントつきは invalid", "a@b.c (comment)", []string{invalid}},
		{"引用されたローカル部は invalid", `"a b"@c.d`, []string{invalid}},
		{"連続するドットは invalid", "a..b@c.d", []string{invalid}},
		{"全角の @ は invalid", "a＠b.c", []string{invalid}},
		{"末尾の全角スペースは invalid(net/mail はそのままでは通す)", "a@b.c\u3000", []string{invalid}},
		{"ローカル部の中の全角スペースは invalid", "a\u3000b@c.d", []string{invalid}},
		{"末尾の NBSP は invalid", "a@b.c\u00a0", []string{invalid}},
		{"ゼロ幅スペースを含む値は invalid", "a@b.c\u200b", []string{invalid}},
		{"先頭の BOM は invalid", "\ufeffa@b.c", []string{invalid}},
		{"C1 制御文字を含む値は invalid", "a@b.c\u0085", []string{invalid}},
		{"タブを含む値は invalid", "a\t@b.c", []string{invalid}},
		{"末尾の改行は invalid", "a@b.c\n", []string{invalid}},
		{"末尾のドットは invalid", "a@b.", []string{invalid}},
		{"一般的な形は有効", "alice@example.com", nil},
		{"大文字を含む値は有効(正規化されず、そのまま比較される)", "Alice@Example.COM", nil},
		{"印字できる非 ASCII の文字は有効(国際化されたアドレス)", "ålice@example.com", nil},
		{"プラス記号つきは有効", "alice+tag@example.com", nil},
		{"サブドメインは有効", "a@mail.example.co.jp", nil},
		{"ドメイン部にドットがなくても有効(RFC の完全準拠は目指さない)", "a@localhost", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateEmail(tt.email); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}
