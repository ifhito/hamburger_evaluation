package repository

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// UserSessionRepository は、sqlc 生成のクエリ上で domain.UserSessionRepository(書き込み)を実装する。
type UserSessionRepository struct {
	q *sqlcgen.Queries
}

// NewUserSessionRepository は db(通常は共有の pgx pool またはトランザクション)をラップする。
func NewUserSessionRepository(db sqlcgen.DBTX) *UserSessionRepository {
	return &UserSessionRepository{q: sqlcgen.New(db)}
}

var _ domain.UserSessionRepository = (*UserSessionRepository)(nil)

// CreateUserSession はセッションの記録を保存する。
func (r *UserSessionRepository) CreateUserSession(ctx context.Context, params domain.CreateUserSessionParams) error {
	err := r.q.CreateUserSession(ctx, sqlcgen.CreateUserSessionParams{
		UserID:     params.UserID,
		TokenHash:  params.TokenHash,
		TtlSeconds: params.TTL.Seconds(),
	})
	if err != nil {
		return fmt.Errorf("create user session: %w", err)
	}
	return nil
}

// DiscardUserSessionByTokenHash は、tokenHash のセッションを削除する。
func (r *UserSessionRepository) DiscardUserSessionByTokenHash(ctx context.Context, tokenHash string) error {
	if err := r.q.DeleteUserSessionByTokenHash(ctx, tokenHash); err != nil {
		return fmt.Errorf("discard user session: %w", err)
	}
	return nil
}

// DiscardUserSessionsByUser は、userID のすべてのセッションを削除する。
func (r *UserSessionRepository) DiscardUserSessionsByUser(ctx context.Context, userID string) error {
	if err := r.q.DeleteUserSessionsByUser(ctx, userID); err != nil {
		return fmt.Errorf("discard user sessions by user: %w", err)
	}
	return nil
}

// DiscardExpiredUserSessions は、期限切れのセッションを最大 limit 件まで削除し、削除件数を返す。
func (r *UserSessionRepository) DiscardExpiredUserSessions(ctx context.Context, limit int) (int64, error) {
	n, err := r.q.DeleteExpiredUserSessions(ctx, int32(limit))
	if err != nil {
		return 0, fmt.Errorf("discard expired user sessions: %w", err)
	}
	return n, nil
}
