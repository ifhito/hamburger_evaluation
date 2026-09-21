package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// SignupVerificationRepository は、sqlc 生成のクエリ上で
// domain.SignupVerificationRepository（書き込み）を実装する。1 つの操作は 1 つの文で行い、
// 複数の操作をまとめて 1 つのトランザクションにするのは、この型の呼び出し側（usecase の
// UnitOfWork）の役目である。UnitOfWork の中では、db にトランザクション（pgx.Tx）が渡される。
type SignupVerificationRepository struct {
	q *sqlcgen.Queries
}

// NewSignupVerificationRepository は db（通常は共有の pgx pool。UnitOfWork の中では pgx.Tx）を
// ラップする。
func NewSignupVerificationRepository(db sqlcgen.DBTX) *SignupVerificationRepository {
	return &SignupVerificationRepository{q: sqlcgen.New(db)}
}

var _ domain.SignupVerificationRepository = (*SignupVerificationRepository)(nil)

// CreateSignupVerification は、email（大文字小文字を区別しない）の確認待ちを upsert する。
// 前回の送信から domain.SignupResendInterval 以内なら、DB の文が行を返さないので、
// Accepted=false の結果を返す（何も変えていない）。有効期間は domain.SignupTokenTTL である。
func (r *SignupVerificationRepository) CreateSignupVerification(ctx context.Context, params domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
	row, err := r.q.UpsertSignupVerification(ctx, sqlcgen.UpsertSignupVerificationParams{
		Email:                 params.Email,
		Username:              params.Username,
		PasswordDigest:        params.PasswordDigest,
		TokenHash:             params.TokenHash,
		TtlSeconds:            domain.SignupTokenTTL.Seconds(),
		ResendIntervalSeconds: domain.SignupResendInterval.Seconds(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SignupVerificationReceipt{}, nil
		}
		return domain.SignupVerificationReceipt{}, fmt.Errorf("create signup verification: %w", err)
	}
	return domain.SignupVerificationReceipt{Accepted: true, ID: row.ID, Generation: int(row.Generation)}, nil
}

// LockSignupVerification は、tokenHash の期限内の確認待ちの行を FOR UPDATE で排他ロックする。
// 行の中身は返さない。行が見つからない（期限切れ・存在しない・使用済み）ときは、
// domain.ErrSignupTokenInvalid を返す。トランザクションの中で呼べば、そのトランザクションが
// 終わるまで、同じトークンを扱うほかの処理は待たされる。待っている間に先の処理が確認待ちを
// 削除して確定すると、待っていた側は行が消えているのを見て、同じエラーになる。
func (r *SignupVerificationRepository) LockSignupVerification(ctx context.Context, tokenHash string) error {
	if _, err := r.q.LockSignupVerificationByTokenHash(ctx, tokenHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lock signup verification: %w", domain.ErrSignupTokenInvalid)
		}
		return fmt.Errorf("lock signup verification: %w", err)
	}
	return nil
}

// DiscardSignupVerification は、id の確認待ちを削除する。
func (r *SignupVerificationRepository) DiscardSignupVerification(ctx context.Context, id string) error {
	if err := r.q.DeleteSignupVerification(ctx, id); err != nil {
		return fmt.Errorf("discard signup verification: %w", err)
	}
	return nil
}

// DiscardExpiredSignupVerifications は、期限切れの確認待ちを最大 limit 件まで削除し、
// 削除した件数を返す。ほかの transaction が掴んでいる行は待たずに飛ばす。
func (r *SignupVerificationRepository) DiscardExpiredSignupVerifications(ctx context.Context, limit int) (int64, error) {
	n, err := r.q.DeleteExpiredSignupVerifications(ctx, int32(limit))
	if err != nil {
		return 0, fmt.Errorf("discard expired signup verifications: %w", err)
	}
	return n, nil
}
