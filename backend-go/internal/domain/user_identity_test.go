package domain

import (
	"errors"
	"testing"
)

func TestExternalIdentityValidate(t *testing.T) {
	ok := ExternalIdentity{Provider: ProviderGoogle, ProviderUserID: "sub-1", Email: "alice@example.com", EmailVerified: true, Name: "Alice"}
	cases := []struct {
		name   string
		mutate func(*ExternalIdentity)
		reject bool
	}{
		{"メールが確認済みで、識別の ID もあれば、サインインに使える", func(*ExternalIdentity) {}, false},
		{"メールが確認済みでないと、他人のメールを名乗れるので、拒否する", func(e *ExternalIdentity) { e.EmailVerified = false }, true},
		{"識別の ID(sub)が空だと、拒否する", func(e *ExternalIdentity) { e.ProviderUserID = "" }, true},
		{"メールが空だと、拒否する", func(e *ExternalIdentity) { e.Email = "" }, true},
		{"メールの形が規則に合わないと、拒否する", func(e *ExternalIdentity) { e.Email = "not-an-email" }, true},
		{"サービスの名前が空だと、拒否する", func(e *ExternalIdentity) { e.Provider = "" }, true},
		{"表示名が空でも、サインインには使える", func(e *ExternalIdentity) { e.Name = "" }, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e := ok
			tt.mutate(&e)
			err := e.Validate()
			if tt.reject && !errors.Is(err, ErrExternalIdentityRejected) {
				t.Fatalf("拒否されるはずが、err = %v", err)
			}
			if !tt.reject && err != nil {
				t.Fatalf("使えるはずが、err = %v", err)
			}
		})
	}
}

func TestCanUnlinkIdentity(t *testing.T) {
	cases := []struct {
		name          string
		hasPassword   bool
		identityCount int
		wantErr       bool
	}{
		{"パスワードがあれば、結び付きを解除できる", true, 1, false},
		{"パスワードがなく、ほかの結び付きもあれば、解除できる", false, 2, false},
		{"パスワードがなく、結び付きが 1 つだけなら、サインインできなくなるので解除できない", false, 1, true},
		{"パスワードも結び付きもなければ、解除できない", false, 0, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := CanUnlinkIdentity(tt.hasPassword, tt.identityCount)
			if tt.wantErr != errors.Is(err, ErrCannotUnlinkIdentity) {
				t.Fatalf("err = %v, 解除できない期待 = %v", err, tt.wantErr)
			}
		})
	}
}
