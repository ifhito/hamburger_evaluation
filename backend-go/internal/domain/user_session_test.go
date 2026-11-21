package domain_test

import (
	"context"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

type recordingSessionRepo struct {
	created          []domain.CreateUserSessionParams
	discardedHashes  []string
	discardedUserIDs []string
	expiredLimits    []int
}

func (r *recordingSessionRepo) CreateUserSession(_ context.Context, params domain.CreateUserSessionParams) error {
	r.created = append(r.created, params)
	return nil
}

func (r *recordingSessionRepo) DiscardUserSessionByTokenHash(_ context.Context, tokenHash string) error {
	r.discardedHashes = append(r.discardedHashes, tokenHash)
	return nil
}

func (r *recordingSessionRepo) DiscardUserSessionsByUser(_ context.Context, userID string) error {
	r.discardedUserIDs = append(r.discardedUserIDs, userID)
	return nil
}

func (r *recordingSessionRepo) DiscardExpiredUserSessions(_ context.Context, limit int) (int64, error) {
	r.expiredLimits = append(r.expiredLimits, limit)
	return 2, nil
}

// TestUserSessions は、書き込みオブジェクトが生トークンを 64 文字の SHA-256 ハッシュへ変換し、
// 平文を repository に渡さないことを検証する。
func TestUserSessions(t *testing.T) {
	ctx := context.Background()
	userID := uid.N(1)
	raw := "raw-jwt-token-value"
	wantHash := domain.HashSessionToken(raw)
	if len(wantHash) != 64 || wantHash == raw {
		t.Fatalf("HashSessionToken = %q (len %d), want 64-char sha256 hex", wantHash, len(wantHash))
	}

	t.Run("セッションを作成すると平文ではなくハッシュと有効期間が保存される", func(t *testing.T) {
		repo := &recordingSessionRepo{}
		sessions := domain.NewUserSessions(repo)
		if err := sessions.Create(ctx, userID, raw, 2*time.Hour); err != nil {
			t.Fatalf("Create error = %v", err)
		}
		if len(repo.created) != 1 {
			t.Fatalf("created = %d, want 1", len(repo.created))
		}
		got := repo.created[0]
		if got.UserID != userID || got.TokenHash != wantHash || got.TTL != 2*time.Hour {
			t.Errorf("created = %+v, want user %s hash %s ttl 2h", got, userID, wantHash)
		}
	})

	t.Run("利用者 ID またはトークンが空のときは保存せずエラーを返す", func(t *testing.T) {
		repo := &recordingSessionRepo{}
		sessions := domain.NewUserSessions(repo)
		if err := sessions.Create(ctx, "", raw, time.Hour); err == nil {
			t.Error("expected error for empty userID")
		}
		if err := sessions.Create(ctx, userID, "", time.Hour); err == nil {
			t.Error("expected error for empty token")
		}
		if err := sessions.Create(ctx, userID, raw, -time.Second); err == nil {
			t.Error("expected error for negative ttl")
		}
		if len(repo.created) != 0 {
			t.Errorf("created = %d, want 0", len(repo.created))
		}
	})

	t.Run("トークン指定の失効と利用者単位の失効がそれぞれ対応する操作を呼ぶ", func(t *testing.T) {
		repo := &recordingSessionRepo{}
		sessions := domain.NewUserSessions(repo)
		if err := sessions.DiscardByToken(ctx, raw); err != nil {
			t.Fatalf("DiscardByToken error = %v", err)
		}
		if len(repo.discardedHashes) != 1 || repo.discardedHashes[0] != wantHash {
			t.Errorf("discardedHashes = %v, want [%s]", repo.discardedHashes, wantHash)
		}
		if err := sessions.DiscardAllByUser(ctx, userID); err != nil {
			t.Fatalf("DiscardAllByUser error = %v", err)
		}
		if len(repo.discardedUserIDs) != 1 || repo.discardedUserIDs[0] != userID {
			t.Errorf("discardedUserIDs = %v, want [%s]", repo.discardedUserIDs, userID)
		}
	})
}
