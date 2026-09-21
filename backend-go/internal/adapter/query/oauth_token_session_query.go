package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// OAuthTokenSessionQuery は、sqlc 生成のクエリ上で usecase.OAuthTokenSessionQuery を実装する。
type OAuthTokenSessionQuery struct {
	q *sqlcgen.Queries
}

// NewOAuthTokenSessionQuery は db（通常は共有の pgx pool）をラップする。
func NewOAuthTokenSessionQuery(db sqlcgen.DBTX) *OAuthTokenSessionQuery {
	return &OAuthTokenSessionQuery{q: sqlcgen.New(db)}
}

var _ usecase.OAuthTokenSessionQuery = (*OAuthTokenSessionQuery)(nil)

// GetOAuthTokenSession は、kind と signature の記録を返す。無効になった記録も返し、なければ
// domain.ErrOAuthTokenSessionNotFound を返す。
func (r *OAuthTokenSessionQuery) GetOAuthTokenSession(ctx context.Context, kind domain.OAuthTokenKind, signature string) (domain.OAuthTokenSession, error) {
	row, err := r.q.GetOAuthTokenSession(ctx, sqlcgen.GetOAuthTokenSessionParams{Kind: string(kind), Signature: signature})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OAuthTokenSession{}, fmt.Errorf("get oauth token session: %w", domain.ErrOAuthTokenSessionNotFound)
		}
		return domain.OAuthTokenSession{}, fmt.Errorf("get oauth token session: %w", err)
	}
	return domain.OAuthTokenSession{
		Kind:      domain.OAuthTokenKind(row.Kind),
		Signature: row.Signature,
		RequestID: row.RequestID,
		GrantID:   row.GrantID,
		UserID:    row.UserID,
		ClientID:  row.ClientID,
		Active:    row.Active,
		ExpiresAt: row.ExpiresAt.Time,
		Request:   row.Request,
	}, nil
}
