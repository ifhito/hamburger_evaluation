package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// SignupVerificationRepository は、sqlc 生成のクエリ上で
// domain.SignupVerificationRepository（書き込み）を実装する。確認（users の作成と
// 確認待ちの削除）は 1 つの transaction で行うので、接続は Begin できなければならない。
type SignupVerificationRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewSignupVerificationRepository は db（通常は共有の pgx pool）をラップする。
func NewSignupVerificationRepository(db beginnerDBTX) *SignupVerificationRepository {
	return &SignupVerificationRepository{db: db, q: sqlcgen.New(db)}
}

var _ domain.SignupVerificationRepository = (*SignupVerificationRepository)(nil)

// CreateSignupVerification は、email（大文字小文字を区別しない）の確認待ちを upsert する。
// 前回の送信から domain.SignupResendInterval 以内なら、DB の文が行を返さないので、
// accepted=false を返す（何も変えていない）。
func (r *SignupVerificationRepository) CreateSignupVerification(ctx context.Context, params domain.CreateSignupVerificationParams) (bool, error) {
	_, err := r.q.UpsertSignupVerification(ctx, sqlcgen.UpsertSignupVerificationParams{
		Email:                 params.Email,
		Username:              params.Username,
		PasswordDigest:        params.PasswordDigest,
		TokenHash:             params.TokenHash,
		TtlSeconds:            params.TTL.Seconds(),
		ResendIntervalSeconds: domain.SignupResendInterval.Seconds(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("create signup verification: %w", err)
	}
	return true, nil
}

// CreateUserFromSignupVerification は、tokenHash の期限内の確認待ちをロックし、その内容で
// users を作成し、確認待ちを削除する。すべて 1 つの transaction で行う。行が見つからない
// （期限切れ・存在しない・使用済み）ときは domain.ErrSignupTokenInvalid、同じ email の users が
// すでにあるとき（unique violation）は domain.ErrEmailTaken を返し、どちらも rollback される。
// 確認待ちを先にロックするので、同じトークンでの並行する確認は直列になり、2 件目は
// ErrSignupTokenInvalid になる。
func (r *SignupVerificationRepository) CreateUserFromSignupVerification(ctx context.Context, tokenHash string) (domain.User, error) {
	var user domain.User
	err := withTx(ctx, r.db, "confirm signup", func(q *sqlcgen.Queries) error {
		pending, err := q.LockSignupVerificationByTokenHash(ctx, tokenHash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("confirm signup: %w", domain.ErrSignupTokenInvalid)
			}
			return fmt.Errorf("confirm signup: lock verification: %w", err)
		}
		row, err := q.CreateUser(ctx, sqlcgen.CreateUserParams{
			Email:          pending.Email,
			Username:       pending.Username,
			PasswordDigest: pending.PasswordDigest,
			// 新しいユーザーは決して admin にならない。昇格は signup の範囲外である。
			Admin: false,
		})
		if err != nil {
			return fmt.Errorf("confirm signup: create user: %w", mapUserWriteError(err))
		}
		if err := q.DeleteSignupVerification(ctx, pending.ID); err != nil {
			return fmt.Errorf("confirm signup: delete verification: %w", err)
		}
		user = rowmap.User(row)
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
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
