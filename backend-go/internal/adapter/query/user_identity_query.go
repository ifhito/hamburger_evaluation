package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// UserIdentityQuery は、sqlc 生成のクエリ上で usecase.IdentityQuery を実装する。ストレージの詳細は
// この境界の内側にとどまり、呼び出し側には domain の型とエラーしか見えない。
type UserIdentityQuery struct {
	q *sqlcgen.Queries
}

// NewUserIdentityQuery は db(通常は共有の pgx pool)をラップする。
func NewUserIdentityQuery(db sqlcgen.DBTX) *UserIdentityQuery {
	return &UserIdentityQuery{q: sqlcgen.New(db)}
}

var _ usecase.IdentityQuery = (*UserIdentityQuery)(nil)

// GetIdentityByProviderUserID は、provider の subject に結び付いた記録を返す。なければ domain.ErrIdentityNotFound を返す。
func (r *UserIdentityQuery) GetIdentityByProviderUserID(ctx context.Context, provider, subject string) (domain.UserIdentity, error) {
	row, err := r.q.GetUserIdentityByProviderUserID(ctx, sqlcgen.GetUserIdentityByProviderUserIDParams{Provider: provider, ProviderUserID: subject})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserIdentity{}, fmt.Errorf("get identity by subject: %w", domain.ErrIdentityNotFound)
		}
		return domain.UserIdentity{}, fmt.Errorf("get identity by subject: %w", err)
	}
	return rowmap.UserIdentity(row), nil
}

// ListIdentitiesByUser は、利用者に結び付いた記録を、結び付けた順に返す。
func (r *UserIdentityQuery) ListIdentitiesByUser(ctx context.Context, userID string) ([]domain.UserIdentity, error) {
	rows, err := r.q.ListUserIdentitiesByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list identities by user: %w", err)
	}
	out := make([]domain.UserIdentity, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowmap.UserIdentity(row))
	}
	return out, nil
}
