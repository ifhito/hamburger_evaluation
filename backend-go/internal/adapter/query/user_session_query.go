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

// UserSessionQuery は、sqlc 生成のクエリ上で usecase.UserSessionQuery を実装する。
type UserSessionQuery struct {
	q *sqlcgen.Queries
}

// NewUserSessionQuery は db(通常は共有の pgx pool)をラップする。
func NewUserSessionQuery(db sqlcgen.DBTX) *UserSessionQuery {
	return &UserSessionQuery{q: sqlcgen.New(db)}
}

var _ usecase.UserSessionQuery = (*UserSessionQuery)(nil)

// GetActiveUserBySessionTokenHash は、有効期限内のセッションと退会していない利用者を 1 回の JOIN で返す。
// 該当がなければ domain.ErrUserSessionNotFound を返す。
func (r *UserSessionQuery) GetActiveUserBySessionTokenHash(ctx context.Context, userID, tokenHash string) (domain.User, error) {
	row, err := r.q.GetActiveUserBySessionTokenHash(ctx, sqlcgen.GetActiveUserBySessionTokenHashParams{
		TokenHash: tokenHash,
		UserID:    userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, fmt.Errorf("get active user by session: %w", domain.ErrUserSessionNotFound)
		}
		return domain.User{}, fmt.Errorf("get active user by session: %w", err)
	}
	return rowmap.SessionUser(row.ID, row.Username, row.Bio, row.Email, row.Admin), nil
}
