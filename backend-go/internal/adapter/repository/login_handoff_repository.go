package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// LoginHandoffRepository は、sqlc 生成のクエリ上で domain.LoginHandoffRepository(書き込み)を実装する。
// 1 つの操作は 1 つの文で行う。
type LoginHandoffRepository struct {
	q *sqlcgen.Queries
}

// NewLoginHandoffRepository は db(通常は共有の pgx pool)をラップする。
func NewLoginHandoffRepository(db sqlcgen.DBTX) *LoginHandoffRepository {
	return &LoginHandoffRepository{q: sqlcgen.New(db)}
}

var _ domain.LoginHandoffRepository = (*LoginHandoffRepository)(nil)

// CreateLoginHandoff はコードの中身を保存する。有効期間は domain.LoginHandoffTTL である。
func (r *LoginHandoffRepository) CreateLoginHandoff(ctx context.Context, params domain.CreateLoginHandoffParams) error {
	var userID *string
	if params.UserID != "" {
		userID = &params.UserID
	}
	err := r.q.CreateLoginHandoff(ctx, sqlcgen.CreateLoginHandoffParams{
		CodeHash:   params.CodeHash,
		Outcome:    string(params.Outcome),
		UserID:     userID,
		ReturnTo:   params.ReturnTo,
		TtlSeconds: domain.LoginHandoffTTL.Seconds(),
	})
	if err != nil {
		return fmt.Errorf("create login handoff: %w", err)
	}
	return nil
}

// DiscardLoginHandoff は、codeHash の、期限内のコードの中身を、削除しながら返す。1 つの文なので、
// 並行して同じコードを使っても、成功するのは 1 回だけである。なければ domain.ErrLoginHandoffInvalid を返す。
func (r *LoginHandoffRepository) DiscardLoginHandoff(ctx context.Context, codeHash string) (domain.LoginHandoff, error) {
	row, err := r.q.DeleteLoginHandoffByCodeHash(ctx, codeHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.LoginHandoff{}, fmt.Errorf("discard login handoff: %w", domain.ErrLoginHandoffInvalid)
		}
		return domain.LoginHandoff{}, fmt.Errorf("discard login handoff: %w", err)
	}
	h := domain.LoginHandoff{ID: row.ID, Outcome: domain.LoginHandoffOutcome(row.Outcome), ReturnTo: row.ReturnTo}
	if row.UserID != nil {
		h.UserID = *row.UserID
	}
	return h, nil
}

// DiscardExpiredLoginHandoffs は、期限切れのコードの中身を、最大 limit 件まで削除し、削除した件数を返す。
func (r *LoginHandoffRepository) DiscardExpiredLoginHandoffs(ctx context.Context, limit int) (int64, error) {
	n, err := r.q.DeleteExpiredLoginHandoffs(ctx, int32(limit))
	if err != nil {
		return 0, fmt.Errorf("discard expired login handoffs: %w", err)
	}
	return n, nil
}
