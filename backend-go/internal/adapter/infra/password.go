package infra

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// BcryptPasswordHasher hashes and verifies passwords with bcrypt,
// matching Rails has_secure_password digests. It implements the
// usecase PasswordHasher interface.
type BcryptPasswordHasher struct{}

// Hash returns the bcrypt digest of password at the default cost.
func (BcryptPasswordHasher) Hash(password string) (string, error) {
	digest, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(digest), nil
}

// Compare returns nil when password matches digest.
func (BcryptPasswordHasher) Compare(digest, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(digest), []byte(password))
}
