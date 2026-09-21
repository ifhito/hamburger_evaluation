package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// db/migrations/000013_create_user_identities.up.sql の UNIQUE 制約の名前である。
const (
	identityProviderSubjectConstraint = "user_identities_provider_user_key"
	identityUserProviderConstraint    = "user_identities_user_provider_key"
)

// UserIdentityRepository は、sqlc 生成のクエリ上で domain.UserIdentityRepository(書き込み)を
// 実装する。1 つの操作は 1 つの文で行う。複数の操作をまとめて 1 つのトランザクションにするのは、
// 呼び出し側(usecase の UnitOfWork)の役目で、その中では、db にトランザクション(pgx.Tx)が渡される。
type UserIdentityRepository struct {
	q *sqlcgen.Queries
}

// NewUserIdentityRepository は db(通常は共有の pgx pool。UnitOfWork の中では pgx.Tx)をラップする。
func NewUserIdentityRepository(db sqlcgen.DBTX) *UserIdentityRepository {
	return &UserIdentityRepository{q: sqlcgen.New(db)}
}

var _ domain.UserIdentityRepository = (*UserIdentityRepository)(nil)

// CreateUserIdentity は結び付きを保存して返す。(provider, subject)の UNIQUE 違反は
// domain.ErrIdentityTaken、(user_id, provider)の UNIQUE 違反は domain.ErrIdentityAlreadyLinked に対応づける。
func (r *UserIdentityRepository) CreateUserIdentity(ctx context.Context, params domain.CreateUserIdentityParams) (domain.UserIdentity, error) {
	row, err := r.q.CreateUserIdentity(ctx, sqlcgen.CreateUserIdentityParams{
		UserID:         params.UserID,
		Provider:       params.Provider,
		ProviderUserID: params.ProviderUserID,
		Email:          params.Email,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			switch pgErr.ConstraintName {
			case identityProviderSubjectConstraint:
				return domain.UserIdentity{}, fmt.Errorf("create user identity: %w", domain.ErrIdentityTaken)
			case identityUserProviderConstraint:
				return domain.UserIdentity{}, fmt.Errorf("create user identity: %w", domain.ErrIdentityAlreadyLinked)
			}
		}
		return domain.UserIdentity{}, fmt.Errorf("create user identity: %w", err)
	}
	return rowmap.UserIdentity(row), nil
}

// DiscardUserIdentitiesByUser は、userID の、すべての結び付きを削除する(1 件もなくてもエラーにしない)。
func (r *UserIdentityRepository) DiscardUserIdentitiesByUser(ctx context.Context, userID string) error {
	if err := r.q.DiscardUserIdentitiesByUser(ctx, userID); err != nil {
		return fmt.Errorf("discard user identities by user: %w", err)
	}
	return nil
}

// DiscardUserIdentity は、userID の、provider の結び付きを削除する。なければ domain.ErrIdentityNotFound を返す。
func (r *UserIdentityRepository) DiscardUserIdentity(ctx context.Context, userID, provider string) error {
	n, err := r.q.DiscardUserIdentity(ctx, sqlcgen.DiscardUserIdentityParams{UserID: userID, Provider: provider})
	if err != nil {
		return fmt.Errorf("discard user identity: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("discard user identity: %w", domain.ErrIdentityNotFound)
	}
	return nil
}
