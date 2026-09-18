package infra

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-only-secret"

func TestJWTCodecRoundTrip(t *testing.T) {
	codec := NewJWTCodec(testSecret, time.Hour)
	const userID = int64(42)

	token, err := codec.Issue(userID)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	got, err := codec.Verify(token)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got != userID {
		t.Fatalf("Verify = %d, want %d", got, userID)
	}

	// Rails/frontend parity: the payload must be exactly
	// {"user_id": <number>, "exp": <unix>}.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]json.Number
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(claims) != 2 {
		t.Fatalf("payload has claims %v, want exactly user_id and exp", claims)
	}
	if claims["user_id"].String() != "42" {
		t.Fatalf("user_id claim = %q, want 42", claims["user_id"])
	}
	if _, err := claims["exp"].Int64(); err != nil {
		t.Fatalf("exp claim %q is not an integer: %v", claims["exp"], err)
	}
}

func TestJWTCodecVerifyRejects(t *testing.T) {
	codec := NewJWTCodec(testSecret, time.Hour)
	claims := jwt.MapClaims{"user_id": int64(42), "exp": time.Now().Add(time.Hour).Unix()}

	tests := []struct {
		name  string
		token func(t *testing.T) string
	}{
		{
			name: "expired token",
			token: func(t *testing.T) string {
				expired := NewJWTCodec(testSecret, -time.Minute)
				token, err := expired.Issue(42)
				if err != nil {
					t.Fatalf("Issue returned error: %v", err)
				}
				return token
			},
		},
		{
			name: "tampered signature",
			token: func(t *testing.T) string {
				token, err := codec.Issue(42)
				if err != nil {
					t.Fatalf("Issue returned error: %v", err)
				}
				last := "A"
				if strings.HasSuffix(token, "A") {
					last = "B"
				}
				return token[:len(token)-1] + last
			},
		},
		{
			name: "wrong secret",
			token: func(t *testing.T) string {
				other := NewJWTCodec("another-secret", time.Hour)
				token, err := other.Issue(42)
				if err != nil {
					t.Fatalf("Issue returned error: %v", err)
				}
				return token
			},
		},
		{
			name: "alg none",
			token: func(t *testing.T) string {
				token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
					SignedString(jwt.UnsafeAllowNoneSignatureType)
				if err != nil {
					t.Fatalf("sign none token: %v", err)
				}
				return token
			},
		},
		{
			name: "alg RS256",
			token: func(t *testing.T) string {
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatalf("generate RSA key: %v", err)
				}
				token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
				if err != nil {
					t.Fatalf("sign RS256 token: %v", err)
				}
				return token
			},
		},
		{
			name: "missing exp",
			token: func(t *testing.T) string {
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": int64(42)}).
					SignedString([]byte(testSecret))
				if err != nil {
					t.Fatalf("sign token without exp: %v", err)
				}
				return token
			},
		},
		{
			name: "missing user_id",
			token: func(t *testing.T) string {
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
					jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}).
					SignedString([]byte(testSecret))
				if err != nil {
					t.Fatalf("sign token without user_id: %v", err)
				}
				return token
			},
		},
		{
			name:  "garbage token",
			token: func(*testing.T) string { return "not.a.jwt" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if userID, err := codec.Verify(tt.token(t)); err == nil {
				t.Fatalf("Verify = %d with nil error, want error", userID)
			}
		})
	}
}
