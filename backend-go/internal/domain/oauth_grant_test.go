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
	created []domain.CreateOAuthGrantParams
	revoked [][2]string
	err     error
}

func (f *fakeGrantRepository) CreateOAuthGrant(_ context.Context, p domain.CreateOAuthGrantParams) (string, error) {
	f.created = append(f.created, p)
	return "grant-1", f.err
}

func (f *fakeGrantRepository) DiscardOAuthGrant(_ context.Context, userID, grantID string) error {
	f.revoked = append(f.revoked, [2]string{userID, grantID})
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
