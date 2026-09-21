package domain_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// fakeGrantRepository は、書き込みオブジェクトに渡された内容を記録する、テスト用の repository である。
type fakeGrantRepository struct {
	created    []domain.CreateOAuthGrantParams
	revoked    [][2]string
	revokedAll []string
	err        error
}

func (f *fakeGrantRepository) CreateOAuthGrant(_ context.Context, p domain.CreateOAuthGrantParams) (string, error) {
	f.created = append(f.created, p)
	return "grant-1", f.err
}

func (f *fakeGrantRepository) DiscardOAuthGrant(_ context.Context, userID, grantID string) error {
	f.revoked = append(f.revoked, [2]string{userID, grantID})
	return f.err
}

func (f *fakeGrantRepository) DiscardOAuthGrantsByUser(_ context.Context, userID string) error {
	f.revokedAll = append(f.revokedAll, userID)
	return f.err
}

func TestOAuthGrantsApprove(t *testing.T) {
	client := domain.OAuthClient{ID: "app", Name: "アプリ", RedirectURIs: []string{"https://app.example.com/cb"}}
	t.Run("許可すると、範囲を一覧の順番に整えて保存し、許可の記録の id を返す", func(t *testing.T) {
		repo := &fakeGrantRepository{}
		id, err := domain.NewOAuthGrants(repo).Approve(context.Background(), "u1", client,
			[]string{domain.OAuthScopeWrite, domain.OAuthScopeRead, domain.OAuthScopeRead})
		if err != nil || id != "grant-1" {
			t.Fatalf("Approve = %q, %v", id, err)
		}
		got := repo.created[0]
		if got.UserID != "u1" || got.ClientID != "app" || strings.Join(got.Scopes, " ") != "hamburger:read hamburger:write" {
			t.Errorf("saved = %+v", got)
		}
	})
	t.Run("知らない範囲を含めて許可しようとすると、何も保存せず、範囲の誤りになる", func(t *testing.T) {
		repo := &fakeGrantRepository{}
		_, err := domain.NewOAuthGrants(repo).Approve(context.Background(), "u1", client, []string{"admin"})
		if !errors.Is(err, domain.ErrOAuthInvalidScope) || len(repo.created) != 0 {
			t.Errorf("err = %v, saved = %d", err, len(repo.created))
		}
	})
	t.Run("範囲が空のまま許可しようとすると、何も保存せず、範囲の誤りになる", func(t *testing.T) {
		repo := &fakeGrantRepository{}
		_, err := domain.NewOAuthGrants(repo).Approve(context.Background(), "u1", client, nil)
		if !errors.Is(err, domain.ErrOAuthInvalidScope) || len(repo.created) != 0 {
			t.Errorf("err = %v, saved = %d", err, len(repo.created))
		}
	})
	t.Run("表示名が上限より長いアプリは、上限の文字数で切って保存する", func(t *testing.T) {
		repo := &fakeGrantRepository{}
		long := client
		long.Name = strings.Repeat("あ", domain.MaxOAuthClientNameLength+30)
		if _, err := domain.NewOAuthGrants(repo).Approve(context.Background(), "u1", long, []string{domain.OAuthScopeRead}); err != nil {
			t.Fatal(err)
		}
		if n := len([]rune(repo.created[0].ClientName)); n != domain.MaxOAuthClientNameLength {
			t.Errorf("saved name has %d characters, want %d", n, domain.MaxOAuthClientNameLength)
		}
	})
}

func TestOAuthGrantsRevoke(t *testing.T) {
	repo := &fakeGrantRepository{}
	if err := domain.NewOAuthGrants(repo).Revoke(context.Background(), "u1", "g1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.revoked) != 1 || repo.revoked[0] != [2]string{"u1", "g1"} {
		t.Errorf("revoked = %v, want [[u1 g1]]", repo.revoked)
	}
}

func TestOAuthGrantsRevokeAll(t *testing.T) {
	repo := &fakeGrantRepository{}
	if err := domain.NewOAuthGrants(repo).RevokeAll(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.revokedAll) != 1 || repo.revokedAll[0] != "u1" {
		t.Errorf("revokedAll = %v, want [u1]", repo.revokedAll)
	}
}

func TestOAuthGrantPermits(t *testing.T) {
	grant := domain.OAuthGrant{ID: "g1", UserID: "u1", ClientID: "app", Scopes: []string{domain.OAuthScopeRead}}
	tests := []struct {
		name             string
		userID, clientID string
		scopes           []string
		wantErr          error
	}{
		{"許可の記録の持ち主が、許可済みのアプリに、許可済みの範囲を求めると、通る", "u1", "app", []string{domain.OAuthScopeRead}, nil},
		{"許可済みの範囲の一部だけを求めても、通る", "u1", "app", []string{}, nil},
		{"許可していない書き込みの範囲を含めて求めると、範囲の誤りとして断る", "u1", "app", []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}, domain.ErrOAuthInvalidScope},
		{"別の利用者が、この許可の記録を使おうとすると、見つからないものとして断る", "u2", "app", []string{domain.OAuthScopeRead}, domain.ErrOAuthGrantNotFound},
		{"別のアプリのために、この許可の記録を使おうとすると、見つからないものとして断る", "u1", "other-app", []string{domain.OAuthScopeRead}, domain.ErrOAuthGrantNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := grant.Permits(tt.userID, tt.clientID, tt.scopes)
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Permits = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
