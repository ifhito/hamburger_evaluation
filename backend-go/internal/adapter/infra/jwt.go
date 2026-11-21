package infra

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// jwtAlg は、受け付ける唯一の署名アルゴリズムである。Rails parity のため
// （Rails は HS256 で署名と検証を行う）。
const jwtAlg = "HS256"

// JWTCodec は、payload が
// {"user_id": "<uuid>", "exp": <unix>, "jti": "<uuid>"} である HS256 の JWT を発行・検証する。
// jti に発行ごとの乱数 UUID を入れることで、同じ秒に同じ利用者へ発行されたトークンも一意になる。
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

// TTL は、発行するトークンの有効期間を返す。サーバー側のセッション記録の有効期間を揃えるために使う。
func (c *JWTCodec) TTL() time.Duration {
	return c.ttl
}

// Issue は、現在から ttl 後に期限切れになる userID 用のトークンに署名する。
func (c *JWTCodec) Issue(userID string) (string, error) {
	jti, err := newRandomUUID()
	if err != nil {
		return "", fmt.Errorf("generate token jti: %w", err)
	}
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(c.ttl).Unix(),
		"jti":     jti,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

// newRandomUUID は RFC 4122 version 4 の乱数 UUID を正規形の小文字文字列で返す。
func newRandomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
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
