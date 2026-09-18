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
)

// ValidationError carries Rails-style full validation messages
// (e.g. "Username can't be blank") for API parity.
type ValidationError struct {
	Messages []string
}

func (e *ValidationError) Error() string {
	return "validation failed: " + strings.Join(e.Messages, ", ")
}
