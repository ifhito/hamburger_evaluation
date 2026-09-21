package domain_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// fakeTokenSessionRepository は、呼ばれた操作を順番に記録する、テスト用の repository である。
type fakeTokenSessionRepository struct {
	calls   []string
	created []domain.OAuthTokenSession
}

func (f *fakeTokenSessionRepository) CreateOAuthTokenSession(_ context.Context, s domain.OAuthTokenSession) error {
	f.calls = append(f.calls, "create")
	f.created = append(f.created, s)
	return nil
}

func (f *fakeTokenSessionRepository) UpdateOAuthTokenSessionInactive(_ context.Context, kind domain.OAuthTokenKind, _ string) error {
	f.calls = append(f.calls, "inactive:"+string(kind))
	return nil
}

func (f *fakeTokenSessionRepository) UpdateOAuthRefreshRotated(context.Context, string, string) error {
	f.calls = append(f.calls, "rotate")
	return nil
}

func (f *fakeTokenSessionRepository) UpdateOAuthTokenSessionsInactiveByRequest(_ context.Context, _ string, kind domain.OAuthTokenKind) error {
	f.calls = append(f.calls, "inactive-by-request:"+string(kind))
	return nil
}

func (f *fakeTokenSessionRepository) DiscardOAuthTokenSession(_ context.Context, kind domain.OAuthTokenKind, _ string) error {
	f.calls = append(f.calls, "discard:"+string(kind))
	return nil
}

func (f *fakeTokenSessionRepository) DiscardOAuthTokenSessionsByRequest(_ context.Context, _ string, kind domain.OAuthTokenKind) error {
	f.calls = append(f.calls, "discard-by-request:"+string(kind))
	return nil
}

func (f *fakeTokenSessionRepository) DiscardExpiredOAuthTokenSessions(context.Context, int) (int64, error) {
	f.calls = append(f.calls, "discard-expired")
	return 0, nil
}

func TestOAuthTokenSessions(t *testing.T) {
	ctx := context.Background()
	t.Run("記録を保存するときは、呼び出し側が指定しなくても、有効な状態で保存する", func(t *testing.T) {
		repo := &fakeTokenSessionRepository{}
		if err := domain.NewOAuthTokenSessions(repo).Save(ctx, domain.OAuthTokenSession{Kind: domain.OAuthTokenAccess, Signature: "s", Active: false}); err != nil {
			t.Fatal(err)
		}
		if len(repo.created) != 1 || !repo.created[0].Active {
			t.Errorf("created = %+v, want one active session", repo.created)
		}
	})
	t.Run("系列を取り消すと、アクセストークンは削除し、更新トークンは記録を残して無効にする", func(t *testing.T) {
		repo := &fakeTokenSessionRepository{}
		if err := domain.NewOAuthTokenSessions(repo).RevokeRequest(ctx, "req"); err != nil {
			t.Fatal(err)
		}
		want := []string{"discard-by-request:access_token", "inactive-by-request:refresh_token"}
		if !reflect.DeepEqual(repo.calls, want) {
			t.Errorf("calls = %v, want %v", repo.calls, want)
		}
	})
	t.Run("認可コードを使うと、その種類の記録を無効にする", func(t *testing.T) {
		repo := &fakeTokenSessionRepository{}
		if err := domain.NewOAuthTokenSessions(repo).Use(ctx, domain.OAuthTokenAuthorizationCode, "sig"); err != nil {
			t.Fatal(err)
		}
		if want := []string{"inactive:authorization_code"}; !reflect.DeepEqual(repo.calls, want) {
			t.Errorf("calls = %v, want %v", repo.calls, want)
		}
	})
}
