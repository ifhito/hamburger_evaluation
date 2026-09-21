package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// OAuthTokenSessionRepository は、sqlc 生成のクエリ上で domain.OAuthTokenSessionRepository（書き込み）を
// 実装する。
type OAuthTokenSessionRepository struct {
	q *sqlcgen.Queries
}

// NewOAuthTokenSessionRepository は db（通常は共有の pgx pool）をラップする。
func NewOAuthTokenSessionRepository(db sqlcgen.DBTX) *OAuthTokenSessionRepository {
	return &OAuthTokenSessionRepository{q: sqlcgen.New(db)}
}

var _ domain.OAuthTokenSessionRepository = (*OAuthTokenSessionRepository)(nil)

// CreateOAuthTokenSession は、記録を保存する。
func (r *OAuthTokenSessionRepository) CreateOAuthTokenSession(ctx context.Context, s domain.OAuthTokenSession) error {
	err := r.q.InsertOAuthTokenSession(ctx, sqlcgen.InsertOAuthTokenSessionParams{
		Kind:      string(s.Kind),
		Signature: s.Signature,
		RequestID: s.RequestID,
		GrantID:   s.GrantID,
		UserID:    s.UserID,
		ClientID:  s.ClientID,
		Active:    s.Active,
		Request:   s.Request,
		ExpiresAt: pgtype.Timestamptz{Time: s.ExpiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create oauth token session: %w", err)
	}
	return nil
}

// UpdateOAuthTokenSessionInactive は、有効な記録を無効にする。判定と更新は 1 つの文で行うので、並行して
// 使おうとしても、成功するのは 1 つだけである。更新できなかったときは、記録がないのか、すでに無効なのかを
// 読み取って区別する(この読み取りは、エラーの種類を決めるためだけで、結果を変えない)。
func (r *OAuthTokenSessionRepository) UpdateOAuthTokenSessionInactive(ctx context.Context, kind domain.OAuthTokenKind, signature string) error {
	n, err := r.q.DeactivateOAuthTokenSession(ctx, sqlcgen.DeactivateOAuthTokenSessionParams{Kind: string(kind), Signature: signature})
	if err != nil {
		return fmt.Errorf("update oauth token session inactive: %w", err)
	}
	if n == 1 {
		return nil
	}
	return r.whyNotUpdated(ctx, kind, signature, "update oauth token session inactive")
}

// UpdateOAuthRefreshRotated は、有効な更新トークンを入れ替え済みにし、同じ系列のアクセストークンを削除する。
// 判定・無効化・削除は 1 つの文で行う。
func (r *OAuthTokenSessionRepository) UpdateOAuthRefreshRotated(ctx context.Context, requestID, signature string) error {
	n, err := r.q.RotateOAuthRefreshToken(ctx, sqlcgen.RotateOAuthRefreshTokenParams{Signature: signature, RequestID: requestID})
	if err != nil {
		return fmt.Errorf("update oauth refresh rotated: %w", err)
	}
	if n == 1 {
		return nil
	}
	return r.whyNotUpdated(ctx, domain.OAuthTokenRefresh, signature, "update oauth refresh rotated")
}

func (r *OAuthTokenSessionRepository) whyNotUpdated(ctx context.Context, kind domain.OAuthTokenKind, signature, op string) error {
	_, err := r.q.GetOAuthTokenSession(ctx, sqlcgen.GetOAuthTokenSessionParams{Kind: string(kind), Signature: signature})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", op, domain.ErrOAuthTokenSessionNotFound)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w", op, domain.ErrOAuthTokenSessionInactive)
}

// UpdateOAuthRequestRevoked は、系列のアクセストークンを削除し、更新トークンを無効にする。1 つの SQL 文で
// 行うので、途中で失敗したときに、片方だけが反映されることはない。
func (r *OAuthTokenSessionRepository) UpdateOAuthRequestRevoked(ctx context.Context, requestID string) error {
	if err := r.q.RevokeOAuthRequest(ctx, requestID); err != nil {
		return fmt.Errorf("update oauth request revoked: %w", err)
	}
	return nil
}

// DiscardOAuthTokenSession は、記録を削除する。なくてもエラーにしない。
func (r *OAuthTokenSessionRepository) DiscardOAuthTokenSession(ctx context.Context, kind domain.OAuthTokenKind, signature string) error {
	if err := r.q.DeleteOAuthTokenSession(ctx, sqlcgen.DeleteOAuthTokenSessionParams{Kind: string(kind), Signature: signature}); err != nil {
		return fmt.Errorf("discard oauth token session: %w", err)
	}
	return nil
}

// DiscardExpiredOAuthTokenSessions は、期限切れの記録を最大 limit 件まで削除し、件数を返す。
// ほかの処理が掴んでいる行は待たずに飛ばす。
func (r *OAuthTokenSessionRepository) DiscardExpiredOAuthTokenSessions(ctx context.Context, limit int) (int64, error) {
	n, err := r.q.DeleteExpiredOAuthTokenSessions(ctx, int32(limit))
	if err != nil {
		return 0, fmt.Errorf("discard expired oauth token sessions: %w", err)
	}
	return n, nil
}
