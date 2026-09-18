package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// usersEmailUniqueConstraint is the users.email UNIQUE constraint name
// from db/migrations/000001_create_users.up.sql.
const usersEmailUniqueConstraint = "users_email_key"

// pgUniqueViolation is SQLSTATE 23505.
const pgUniqueViolation = "23505"

// UserRepository implements usecase.UserRepository over sqlc-generated
// queries. Storage details (sqlc rows, pgtype, pg error codes) stay
// inside this boundary; callers only see domain types and errors.
type UserRepository struct {
	q *sqlcgen.Queries
}

// NewUserRepository wraps db (normally the shared pgx pool).
func NewUserRepository(db sqlcgen.DBTX) *UserRepository {
	return &UserRepository{q: sqlcgen.New(db)}
}

var _ usecase.UserRepository = (*UserRepository)(nil)

// CreateUser inserts a new user and returns it. A unique violation on
// the email column maps to domain.ErrEmailTaken.
func (r *UserRepository) CreateUser(ctx context.Context, params usecase.CreateUserParams) (domain.User, error) {
	row, err := r.q.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email:          params.Email,
		Username:       params.Username,
		PasswordDigest: params.PasswordDigest,
		Admin:          params.Admin,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == usersEmailUniqueConstraint {
			return domain.User{}, fmt.Errorf("create user: %w", domain.ErrEmailTaken)
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return toDomainUser(row), nil
}

// GetActiveUserByEmail returns the non-discarded user with the given
// email together with its password digest, or domain.ErrUserNotFound.
func (r *UserRepository) GetActiveUserByEmail(ctx context.Context, email string) (usecase.UserCredentials, error) {
	row, err := r.q.GetActiveUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return usecase.UserCredentials{}, fmt.Errorf("get active user by email: %w", domain.ErrUserNotFound)
		}
		return usecase.UserCredentials{}, fmt.Errorf("get active user by email: %w", err)
	}
	return usecase.UserCredentials{User: toDomainUser(row), PasswordDigest: row.PasswordDigest}, nil
}

// GetActiveUserByID returns the non-discarded user with the given id,
// or domain.ErrUserNotFound.
func (r *UserRepository) GetActiveUserByID(ctx context.Context, id int64) (domain.User, error) {
	row, err := r.q.GetActiveUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, fmt.Errorf("get active user by id: %w", domain.ErrUserNotFound)
		}
		return domain.User{}, fmt.Errorf("get active user by id: %w", err)
	}
	return toDomainUser(row), nil
}

// toDomainUser maps a sqlc row to the domain entity, dropping the
// password digest and storage-only columns.
func toDomainUser(row sqlcgen.User) domain.User {
	return domain.User{
		ID:       row.ID,
		Username: row.Username,
		Email:    row.Email,
		Admin:    row.Admin,
	}
}
