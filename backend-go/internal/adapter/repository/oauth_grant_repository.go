package repository

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// OAuthGrantRepository は、sqlc 生成のクエリ上で domain.OAuthGrantRepository（書き込み）を実装する。
type OAuthGrantRepository struct {
	q *sqlcgen.Queries
}

// NewOAuthGrantRepository は db（通常は共有の pgx pool。UnitOfWork の中では pgx.Tx）をラップする。
func NewOAuthGrantRepository(db sqlcgen.DBTX) *OAuthGrantRepository {
	return &OAuthGrantRepository{q: sqlcgen.New(db)}
}

var _ domain.OAuthGrantRepository = (*OAuthGrantRepository)(nil)

// CreateOAuthGrant は、利用者とアプリの組の許可を upsert する。すでにあるときは、許可の範囲を、
// 既存の範囲と params の範囲を合わせたものにし(狭めない)、同じ id を返す。
func (r *OAuthGrantRepository) CreateOAuthGrant(ctx context.Context, params domain.CreateOAuthGrantParams) (string, error) {
	id, err := r.q.UpsertOAuthGrant(ctx, sqlcgen.UpsertOAuthGrantParams{
		UserID:     params.UserID,
		ClientID:   params.ClientID,
		ClientName: params.ClientName,
		Scopes:     params.Scopes,
	})
	if err != nil {
		return "", fmt.Errorf("create oauth grant: %w", err)
	}
	return id, nil
}

// DiscardOAuthGrant は、userID の grantID の許可を削除する。発行済みのトークンは、外部キーの連鎖削除で、
// 同時に消える。別の利用者の許可や存在しない許可は、0 行になり、domain.ErrOAuthGrantNotFound を返す。
func (r *OAuthGrantRepository) DiscardOAuthGrant(ctx context.Context, userID, grantID string) error {
	n, err := r.q.DeleteOAuthGrant(ctx, sqlcgen.DeleteOAuthGrantParams{ID: grantID, UserID: userID})
	if err != nil {
		return fmt.Errorf("discard oauth grant: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("discard oauth grant: %w", domain.ErrOAuthGrantNotFound)
	}
	return nil
}

// DiscardOAuthGrantsByUser は、userID のすべての許可を削除する。発行済みのトークンは、外部キーの連鎖削除で、
// 同時に消える。
func (r *OAuthGrantRepository) DiscardOAuthGrantsByUser(ctx context.Context, userID string) error {
	if err := r.q.DeleteOAuthGrantsByUser(ctx, userID); err != nil {
		return fmt.Errorf("discard oauth grants by user: %w", err)
	}
	return nil
}
