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

// SignupVerificationQuery は、sqlc 生成のクエリ上で usecase.SignupVerificationQuery を実装する。
// ストレージの詳細（sqlc の行、pg のエラー）はこの境界の内側にとどまり、呼び出し側には
// domain の型とエラーしか見えない。
type SignupVerificationQuery struct {
	q *sqlcgen.Queries
}

// NewSignupVerificationQuery は db（通常は共有の pgx pool。UnitOfWork の中では pgx.Tx）をラップする。
func NewSignupVerificationQuery(db sqlcgen.DBTX) *SignupVerificationQuery {
	return &SignupVerificationQuery{q: sqlcgen.New(db)}
}

var _ usecase.SignupVerificationQuery = (*SignupVerificationQuery)(nil)

// GetSignupVerificationByTokenHash は、tokenHash の期限内の確認待ちの内容を返す。期限切れ・
// 存在しない・使用済みのときは、domain.ErrSignupTokenInvalid を返す。
func (r *SignupVerificationQuery) GetSignupVerificationByTokenHash(ctx context.Context, tokenHash string) (domain.PendingSignup, error) {
	row, err := r.q.GetSignupVerificationByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PendingSignup{}, fmt.Errorf("get signup verification: %w", domain.ErrSignupTokenInvalid)
		}
		return domain.PendingSignup{}, fmt.Errorf("get signup verification: %w", err)
	}
	return domain.PendingSignup{
		ID:             row.ID,
		Email:          row.Email,
		Username:       row.Username,
		PasswordDigest: row.PasswordDigest,
	}, nil
}
