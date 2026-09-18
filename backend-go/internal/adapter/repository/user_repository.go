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
// inside this boundary; callers only see domain types and errors. The S8
// writes (profile update, user discard) are transactional, so the
// connection must be Begin-capable, like ReviewRepository's.
type UserRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewUserRepository wraps db (normally the shared pgx pool).
func NewUserRepository(db beginnerDBTX) *UserRepository {
	return &UserRepository{db: db, q: sqlcgen.New(db)}
}

var (
	_ usecase.UserRepository  = (*UserRepository)(nil)
	_ usecase.UsersRepository = (*UserRepository)(nil)
)

// ProfileChanges aliases the usecase type (the consumer-side contract
// owns it; the repository merely conforms — dependencies point inward).
type ProfileChanges = usecase.ProfileChanges

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
		return domain.User{}, fmt.Errorf("create user: %w", mapUserWriteError(err))
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

// ListActiveUsers returns every kept user, id ascending (no pagination —
// Rails parity: the index returns all kept users).
func (r *UserRepository) ListActiveUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := r.q.ListActiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active users: %w", err)
	}
	users := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, toDomainUser(row))
	}
	return users, nil
}

// UpdateUserProfile applies the present fields of changes to the still
// kept user under id in one transaction, each field via its own
// column-scoped UPDATE, and returns the stored user. A missing or
// discarded user yields domain.ErrUserNotFound; an email unique violation
// yields domain.ErrEmailTaken (either way the transaction is rolled back,
// so no field is partially applied). Zero present fields are a plain
// lookup of the current user (200 no-op, Rails parity).
func (r *UserRepository) UpdateUserProfile(ctx context.Context, id int64, changes ProfileChanges) (domain.User, error) {
	if changes.Username == nil && changes.Email == nil && changes.PasswordDigest == nil {
		return r.GetActiveUserByID(ctx, id)
	}
	var row sqlcgen.User
	err := withTx(ctx, r.db, "update user profile", func(q *sqlcgen.Queries) error {
		var err error
		if changes.Username != nil {
			row, err = q.UpdateUserUsername(ctx, sqlcgen.UpdateUserUsernameParams{ID: id, Username: *changes.Username})
			if err != nil {
				return fmt.Errorf("update user profile: username: %w", mapUserWriteError(err))
			}
		}
		if changes.Email != nil {
			row, err = q.UpdateUserEmail(ctx, sqlcgen.UpdateUserEmailParams{ID: id, Email: *changes.Email})
			if err != nil {
				return fmt.Errorf("update user profile: email: %w", mapUserWriteError(err))
			}
		}
		if changes.PasswordDigest != nil {
			row, err = q.UpdateUserPasswordDigest(ctx, sqlcgen.UpdateUserPasswordDigestParams{ID: id, PasswordDigest: *changes.PasswordDigest})
			if err != nil {
				return fmt.Errorf("update user profile: password digest: %w", mapUserWriteError(err))
			}
		}
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(row), nil
}

// DiscardUser soft-deletes the user (stamps users.discarded_at, never a
// hard DELETE) and recalculates the burger_stats of every burger the
// user's kept reviews touch, all in one transaction. Missing and
// already-discarded users match no row and yield domain.ErrUserNotFound.
// The user's reviews themselves stay kept (reviews.discarded_at is never
// written — Rails parity; hiding is done by the read-side u.discarded_at
// filters), but the recalculation still drops them from the stats because
// ListBurgerReviewFacts excludes discarded users' reviews.
func (r *UserRepository) DiscardUser(ctx context.Context, id int64) error {
	return withTx(ctx, r.db, "discard user", func(q *sqlcgen.Queries) error {
		if _, err := q.DiscardUser(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("discard user: %w", domain.ErrUserNotFound)
			}
			return fmt.Errorf("discard user: %w", err)
		}
		burgerIDs, err := q.ListUserKeptReviewBurgerIDs(ctx, id)
		if err != nil {
			return fmt.Errorf("discard user: list review burgers: %w", err)
		}
		// The query returns the ids in ascending order — the multi-burger
		// lock-ordering rule of recalculateBurgerStats (deadlock avoidance).
		for _, burgerID := range burgerIDs {
			if err := recalculateBurgerStats(ctx, q, burgerID); err != nil {
				return fmt.Errorf("discard user: %w", err)
			}
		}
		return nil
	})
}

// mapUserWriteError translates the storage errors of user writes into
// domain errors: no matched row (missing or discarded user) becomes
// domain.ErrUserNotFound, a unique violation on users.email becomes
// domain.ErrEmailTaken; anything else passes through unchanged.
func mapUserWriteError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == usersEmailUniqueConstraint {
		return domain.ErrEmailTaken
	}
	return err
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
