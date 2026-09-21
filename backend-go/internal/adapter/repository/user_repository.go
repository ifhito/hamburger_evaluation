package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// usersEmailUniqueConstraint は、db/migrations/000001_create_users.up.sql の
// users.email の UNIQUE 制約の名前である。
const usersEmailUniqueConstraint = "users_email_key"

// pgUniqueViolation は SQLSTATE 23505 である。
const pgUniqueViolation = "23505"

// UserRepository は、sqlc 生成のクエリ上で domain.UserRepository（書き込み）を
// 実装する。ストレージの詳細（sqlc の行、pgtype、pg のエラーコード）はこの境界の
// 内側にとどまり、呼び出し側には domain の型とエラーしか見えない。S8 の書き込み
// （プロフィールの更新、user の discard）はトランザクションで行われるので、
// 接続は ReviewRepository のものと同様に Begin できなければならない。読み取りは
// adapter/query の UserQuery が担う。
type UserRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewUserRepository は db（通常は共有の pgx pool）をラップする。
func NewUserRepository(db beginnerDBTX) *UserRepository {
	return &UserRepository{db: db, q: sqlcgen.New(db)}
}

var _ domain.UserRepository = (*UserRepository)(nil)

// CreateUser は新しい user を insert して返す。email カラムでの unique
// violation は domain.ErrEmailTaken に対応づけられる。
func (r *UserRepository) CreateUser(ctx context.Context, params domain.CreateUserParams) (domain.User, error) {
	row, err := r.q.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email:          params.Email,
		Username:       params.Username,
		PasswordDigest: params.PasswordDigest,
		Admin:          params.Admin,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", mapUserWriteError(err))
	}
	return rowmap.User(row), nil
}

// UpdateUserProfile は、changes のうち指定されているフィールドを、id の、
// まだ kept な user に 1 つのトランザクションで適用する。各フィールドは
// それぞれ専用のカラム単位の UPDATE で更新し、保存された user を返す。
// 存在しないか discard 済みの user は domain.ErrUserNotFound を返し、email の
// unique violation は domain.ErrEmailTaken を返す（どちらの場合も
// トランザクションは rollback されるので、一部のフィールドだけが適用される
// ことはない）。指定されたフィールドがゼロ個の場合は、現在の user を単純に
// 参照するだけである（200 の no-op、Rails parity）。
func (r *UserRepository) UpdateUserProfile(ctx context.Context, id int64, changes domain.ProfileChanges) (domain.User, error) {
	if changes.Username == nil && changes.Email == nil && changes.PasswordDigest == nil {
		return r.activeUserByID(ctx, id)
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
	return rowmap.User(row), nil
}

// DiscardUser は user を soft delete し（users.discarded_at に時刻を刻み、
// hard DELETE は決して行わない）、その user の kept な review が触れている
// すべての burger の burger_stats を再計算する。すべて 1 つの
// トランザクションで行う。存在しない user や、すでに discard 済みの user は
// どの行にも一致せず、domain.ErrUserNotFound を返す。user の review 自体は
// kept のままである（reviews.discarded_at は決して書き込まれない。
// Rails parity。非表示化は読み取り側の u.discarded_at フィルタで行う）。
// ただし、ListBurgerReviewFacts が discard 済みの user の review を除外する
// ので、再計算によって、それらの review は stats から外れる。
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
		// クエリは id を昇順で返す。これは recalculateBurgerStats の、複数
		// burger にわたるロック順序のルールである（デッドロックの回避）。
		for _, burgerID := range burgerIDs {
			if err := recalculateBurgerStats(ctx, q, burgerID); err != nil {
				return fmt.Errorf("discard user: %w", err)
			}
		}
		return nil
	})
}

// mapUserWriteError は、user の書き込みで生じたストレージのエラーを domain の
// エラーに変換する。行が一致しなかった場合（存在しない、または discard 済みの
// user）は domain.ErrUserNotFound に、users.email の unique violation は
// domain.ErrEmailTaken になり、それ以外はそのまま素通りする。
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

// activeUserByID は、指定された id を持つ discard されていない user を返す。
// または domain.ErrUserNotFound を返す。UpdateUserProfile の no-op（変更する
// フィールドがゼロ個の場合）のための、書き込みの内部の lookup であり、
// domain.UserRepository には含まれない（読み取りは usecase.UserQuery）。
func (r *UserRepository) activeUserByID(ctx context.Context, id int64) (domain.User, error) {
	row, err := r.q.GetActiveUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, fmt.Errorf("get active user by id: %w", domain.ErrUserNotFound)
		}
		return domain.User{}, fmt.Errorf("get active user by id: %w", err)
	}
	return rowmap.User(row), nil
}
