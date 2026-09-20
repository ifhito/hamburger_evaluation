package infra

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtAlg is the only accepted signing algorithm, for Rails parity
// (Rails signs and verifies with HS256).
const jwtAlg = "HS256"

// JWTCodec issues and verifies HS256 JWTs whose payload is exactly
// {"user_id": <number>, "exp": <unix>}, matching the Rails backend and
// the frontend, which decodes payload.user_id. It implements the
// usecase TokenIssuer and TokenVerifier interfaces. Secrets and tokens
// must never be logged.
type JWTCodec struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTCodec builds a codec from the configured secret and token TTL.
func NewJWTCodec(secret string, ttl time.Duration) *JWTCodec {
	return &JWTCodec{secret: []byte(secret), ttl: ttl}
}

// Issue signs a token for userID expiring ttl from now.
func (c *JWTCodec) Issue(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(c.ttl).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

// Verify parses raw, pinning the algorithm to HS256 and requiring a
// valid exp claim, and returns the user_id claim.
func (c *JWTCodec) Verify(raw string) (int64, error) {
	token, err := jwt.Parse(raw,
		func(*jwt.Token) (any, error) { return c.secret, nil },
		jwt.WithValidMethods([]string{jwtAlg}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return 0, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, fmt.Errorf("unexpected claims type %T", token.Claims)
	}
	// JSON numbers decode as float64.
	userID, ok := claims["user_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("token has no numeric user_id claim")
	}
	return int64(userID), nil
}
