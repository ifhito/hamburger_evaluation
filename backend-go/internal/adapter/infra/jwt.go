package infra

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtAlg は、受け付ける唯一の署名アルゴリズムである。Rails parity のため
// （Rails は HS256 で署名と検証を行う）。
const jwtAlg = "HS256"

// JWTCodec は、payload がちょうど
// {"user_id": <number>, "exp": <unix>} である HS256 の JWT を発行・検証する。
// これは Rails バックエンドおよび payload.user_id をデコードする frontend と
// 一致している。usecase の TokenIssuer と TokenVerifier の interface を
// 実装する。secret とトークンは決してログに出力してはならない。
type JWTCodec struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTCodec は、設定された secret とトークンの TTL から codec を構築する。
func NewJWTCodec(secret string, ttl time.Duration) *JWTCodec {
	return &JWTCodec{secret: []byte(secret), ttl: ttl}
}

// Issue は、現在から ttl 後に期限切れになる userID 用のトークンに署名する。
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

// Verify は raw をパースする。アルゴリズムを HS256 に固定し、有効な exp
// claim を必須とし、user_id claim を返す。
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
	// JSON の数値は float64 としてデコードされる。
	userID, ok := claims["user_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("token has no numeric user_id claim")
	}
	return int64(userID), nil
}
