package domain

import (
	"errors"
	"strings"
)

// Sentinel authentication errors. Callers match them with errors.Is.
var (
	// ErrInvalidCredentials signals a login failure. Unknown email and
	// wrong password intentionally map to the same error so callers
	// cannot distinguish which part failed (no user enumeration).
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrUnauthenticated signals that a token is missing, invalid,
	// expired, or belongs to an unknown or discarded user.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrEmailTaken signals a unique violation on the user email.
	ErrEmailTaken = errors.New("email has already been taken")
	// ErrUserNotFound signals that no active user matches the lookup.
	ErrUserNotFound = errors.New("user not found")
	// ErrShopNotFound signals that no shop matches the lookup or that the
	// viewer may not see it; the two cases are deliberately identical so
	// hidden shops' existence is not leaked.
	ErrShopNotFound = errors.New("shop not found")
	// ErrReviewNotFound signals that no non-discarded review matches the
	// lookup; missing and soft-deleted reviews are deliberately identical.
	ErrReviewNotFound = errors.New("review not found")
	// ErrBurgerNotFound signals that no burger matches the lookup within
	// the requested shop (unknown burger and burger of another shop are
	// deliberately identical).
	ErrBurgerNotFound = errors.New("burger not found")
	// ErrForbidden signals that the viewer is authenticated but not
	// allowed to perform the operation (e.g. a non-admin calling a
	// moderation use case).
	ErrForbidden = errors.New("forbidden")
)

// ValidationError carries Rails-style full validation messages
// (e.g. "Username can't be blank") for API parity.
type ValidationError struct {
	Messages []string
}

func (e *ValidationError) Error() string {
	return "validation failed: " + strings.Join(e.Messages, ", ")
}
