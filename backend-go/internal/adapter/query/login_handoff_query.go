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

// LoginHandoffQuery は、sqlc 生成のクエリ上で usecase.LoginHandoffQuery を実装する。UnitOfWork の中では、
// トランザクションに結び付いたものが渡されるので、同じトランザクションでロックした行を、そのまま読める。
type LoginHandoffQuery struct {
	q *sqlcgen.Queries
}

// NewLoginHandoffQuery は db(通常は共有の pgx pool。UnitOfWork の中では pgx.Tx)をラップする。
func NewLoginHandoffQuery(db sqlcgen.DBTX) *LoginHandoffQuery {
	return &LoginHandoffQuery{q: sqlcgen.New(db)}
}

var _ usecase.LoginHandoffQuery = (*LoginHandoffQuery)(nil)

// GetLoginHandoffByCodeHash は、codeHash の、期限内のコードの中身を返す。なければ
// domain.ErrLoginHandoffInvalid を返す。
func (r *LoginHandoffQuery) GetLoginHandoffByCodeHash(ctx context.Context, codeHash string) (domain.LoginHandoff, error) {
	row, err := r.q.GetLoginHandoffByCodeHash(ctx, codeHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.LoginHandoff{}, fmt.Errorf("get login handoff: %w", domain.ErrLoginHandoffInvalid)
		}
		return domain.LoginHandoff{}, fmt.Errorf("get login handoff: %w", err)
	}
	h := domain.LoginHandoff{ID: row.ID, Outcome: domain.LoginHandoffOutcome(row.Outcome), ReturnTo: row.ReturnTo, BinderHash: row.BinderHash}
	if row.UserID != nil {
		h.UserID = *row.UserID
	}
	return h, nil
}
