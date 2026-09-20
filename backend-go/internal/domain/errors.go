package domain

import (
	"errors"
	"strings"
)

// Sentinel な認証エラー群。呼び出し側は errors.Is で照合する。
var (
	// ErrInvalidCredentials はログインの失敗を表す。未知の email と誤った
	// パスワードは意図的に同じエラーへ対応させており、呼び出し側はどちらが
	// 失敗したのかを区別できない（user enumeration を防ぐ）。
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrUnauthenticated は、トークンが存在しない、無効である、期限切れで
	// ある、または未知のユーザーか discard 済みのユーザーに属していることを
	// 表す。
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrEmailTaken は、ユーザーの email に対する unique violation を表す。
	ErrEmailTaken = errors.New("email has already been taken")
	// ErrUserNotFound は、lookup に一致する active なユーザーがいないことを
	// 表す。
	ErrUserNotFound = errors.New("user not found")
	// ErrShopNotFound は、lookup に一致する shop がない、または viewer には
	// その shop を見る権限がないことを表す。この 2 つのケースは意図的に同一
	// であり、隠された shop の存在が漏れないようにしている。
	ErrShopNotFound = errors.New("shop not found")
	// ErrReviewNotFound は、lookup に一致する non-discarded な review が
	// ないことを表す。存在しない review と soft delete 済みの review は
	// 意図的に同一である。
	ErrReviewNotFound = errors.New("review not found")
	// ErrBurgerNotFound は、指定された shop の中に lookup に一致する burger が
	// ないことを表す（未知の burger と別の shop の burger は意図的に同一で
	// ある）。
	ErrBurgerNotFound = errors.New("burger not found")
	// ErrForbidden は、viewer は認証済みだがその操作を行う権限がないことを
	// 表す（例：admin でないユーザーが moderation の use case を呼び出す
	// 場合）。
	ErrForbidden = errors.New("forbidden")
)

// ValidationError は、API parity のために Rails 形式の full validation
// message（例："Username can't be blank"）を保持する。
type ValidationError struct {
	Messages []string
}

func (e *ValidationError) Error() string {
	return "validation failed: " + strings.Join(e.Messages, ", ")
}
