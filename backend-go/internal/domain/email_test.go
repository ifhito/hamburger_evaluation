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
		{"一般的な形は有効", "alice@example.com", nil},
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
