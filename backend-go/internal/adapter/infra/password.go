package infra

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// BcryptPasswordHasher は bcrypt でパスワードのハッシュ化と検証を行い、
// Rails の has_secure_password の digest と一致する。usecase の
// PasswordHasher interface を実装する。
type BcryptPasswordHasher struct{}

// Hash は、デフォルトの cost での password の bcrypt digest を返す。
func (BcryptPasswordHasher) Hash(password string) (string, error) {
	digest, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(digest), nil
}

// Compare は password が digest に一致するとき nil を返す。
func (BcryptPasswordHasher) Compare(digest, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(digest), []byte(password))
}
