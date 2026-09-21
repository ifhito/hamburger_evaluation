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

// UserQuery は、sqlc 生成のクエリ上で usecase.UserQuery を実装する。
// ストレージの詳細（sqlc の行、pg のエラー）はこの境界の内側にとどまり、
// 呼び出し側には domain の型とエラーしか見えない。
type UserQuery struct {
	q *sqlcgen.Queries
}

// NewUserQuery は db（通常は共有の pgx pool）をラップする。
func NewUserQuery(db sqlcgen.DBTX) *UserQuery {
	return &UserQuery{q: sqlcgen.New(db)}
}

var _ usecase.UserQuery = (*UserQuery)(nil)

// GetActiveUserByEmail は、指定された email を持つ discard されていない
// user をその password digest とともに返す。パスワードを持たないアカウントの digest は空文字列である。
// 該当がなければ domain.ErrUserNotFound を返す。
func (r *UserQuery) GetActiveUserByEmail(ctx context.Context, email string) (usecase.UserCredentials, error) {
	row, err := r.q.GetActiveUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return usecase.UserCredentials{}, fmt.Errorf("get active user by email: %w", domain.ErrUserNotFound)
		}
		return usecase.UserCredentials{}, fmt.Errorf("get active user by email: %w", err)
	}
	return usecase.UserCredentials{User: rowmap.User(row), PasswordDigest: row.PasswordDigest.String}, nil
}

// GetActiveUserByID は、指定された id を持つ discard されていない user を
// 返す。または domain.ErrUserNotFound を返す。
func (r *UserQuery) GetActiveUserByID(ctx context.Context, id string) (domain.User, error) {
	row, err := r.q.GetActiveUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, fmt.Errorf("get active user by id: %w", domain.ErrUserNotFound)
		}
		return domain.User{}, fmt.Errorf("get active user by id: %w", err)
	}
	return rowmap.User(row), nil
}
