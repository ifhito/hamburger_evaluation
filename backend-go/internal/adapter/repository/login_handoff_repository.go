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
// 1 つの操作は 1 つの文で行い、複数の操作をまとめて 1 つのトランザクションにするのは、呼び出し側(usecase の
// UnitOfWork)の役目である(その中では、db にトランザクション(pgx.Tx)が渡される)。
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
		BinderHash: params.BinderHash,
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

// LockLoginHandoff は、codeHash の、期限内のコードの行を FOR UPDATE で排他ロックする。行の中身は返さない。
// 行がない(期限切れ・存在しない・使用済み)ときは、domain.ErrLoginHandoffInvalid を返す。トランザクションの
// 中で呼べば、そのトランザクションが終わるまで、同じコードを使うほかの処理は待たされる。待っている間に先の処理が
// コードを削除して確定すると、待っていた側は行が消えているのを見て、同じエラーになる。
func (r *LoginHandoffRepository) LockLoginHandoff(ctx context.Context, codeHash string) error {
	if _, err := r.q.LockLoginHandoffByCodeHash(ctx, codeHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lock login handoff: %w", domain.ErrLoginHandoffInvalid)
		}
		return fmt.Errorf("lock login handoff: %w", err)
	}
	return nil
}

// DiscardLoginHandoff は、id のコードの行を削除する。
func (r *LoginHandoffRepository) DiscardLoginHandoff(ctx context.Context, id string) error {
	if err := r.q.DeleteLoginHandoff(ctx, id); err != nil {
		return fmt.Errorf("discard login handoff: %w", err)
	}
	return nil
}

// DiscardExpiredLoginHandoffs は、期限切れのコードの中身を、最大 limit 件まで削除し、削除した件数を返す。
func (r *LoginHandoffRepository) DiscardExpiredLoginHandoffs(ctx context.Context, limit int) (int64, error) {
	n, err := r.q.DeleteExpiredLoginHandoffs(ctx, int32(limit))
	if err != nil {
		return 0, fmt.Errorf("discard expired login handoffs: %w", err)
	}
	return n, nil
}
