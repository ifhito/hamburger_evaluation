package infra

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// jwtAlg は、受け付ける唯一の署名アルゴリズムである。Rails parity のため
// （Rails は HS256 で署名と検証を行う）。
const jwtAlg = "HS256"

// JWTCodec は、payload がちょうど
// {"user_id": "<uuid>", "exp": <unix>} である HS256 の JWT を発行・検証する。
// frontend の AuthProvider は payload をデコードするが、期限切れの判定に使うのは
// exp だけである。usecase の TokenIssuer と TokenVerifier の interface を実装する。
// secret とトークンは決してログに出力してはならない。
type JWTCodec struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTCodec は、設定された secret とトークンの TTL から codec を構築する。
func NewJWTCodec(secret string, ttl time.Duration) *JWTCodec {
	return &JWTCodec{secret: []byte(secret), ttl: ttl}
}

// Issue は、現在から ttl 後に期限切れになる userID 用のトークンに署名する。
func (c *JWTCodec) Issue(userID string) (string, error) {
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

// Verify は raw をパースする。アルゴリズムを HS256 に固定し、有効な exp
// claim を必須とし、user_id claim（UUID の正規形の文字列）を返す。数値の
// user_id を持つ旧形式のトークンは、文字列でないので無効になる。
func (c *JWTCodec) Verify(raw string) (string, error) {
	token, err := jwt.Parse(raw,
		func(*jwt.Token) (any, error) { return c.secret, nil },
		jwt.WithValidMethods([]string{jwtAlg}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("unexpected claims type %T", token.Claims)
	}
	userID, ok := claims["user_id"].(string)
	if !ok || !domain.IsUUID(userID) {
		return "", fmt.Errorf("token has no uuid user_id claim")
	}
	return userID, nil
}
