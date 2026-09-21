package domain

import (
	"reflect"
	"testing"
)

func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		want     []string
	}{
		{"有効な認証情報は違反なし", "alice@example.com", "Password123!", nil},
		{"email と password が空なら両方の blank を返す", "", "", []string{"Email can't be blank", "Password can't be blank"}},
		{"email の違反が password の違反より先に並ぶ", "abc", "short", []string{
			"Email is invalid",
			"Password is too short (minimum is 8 characters)",
			"Password must include letters, numbers and symbols",
		}},
		{"email が有効で password だけ違反なら password の違反だけ", "alice@example.com", "password", []string{"Password must include letters, numbers and symbols"}},
		{"password が有効で email だけ違反なら email の違反だけ", "abc", "Password123!", []string{"Email is invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateCredentials(tt.email, tt.password); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidateCredentials(%q, %q) = %v, want %v", tt.email, tt.password, got, tt.want)
			}
		})
	}
}
