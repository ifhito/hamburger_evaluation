// Package infra holds infrastructure concerns: environment configuration,
// database pool construction, JWT issuing/verification, and password
// hashing.
package infra

import (
	"fmt"
	"strconv"
	"time"
)

const (
	defaultPort       = "8080"
	defaultDBMaxConns = int32(10)
	defaultJWTTTL     = 24 * time.Hour
)

// Config is the process configuration loaded from the environment.
type Config struct {
	// Port is the TCP port the HTTP server listens on. PORT, default 8080.
	Port string
	// DatabaseURL is the PostgreSQL connection string. DATABASE_URL, required.
	DatabaseURL string
	// JWTSecret is the HMAC secret for JWT auth. JWT_SECRET, required.
	// Its value must never be hardcoded or logged.
	JWTSecret string
	// JWTTTL is the lifetime of issued JWTs. JWT_TTL (a Go duration such
	// as "24h"), default 24h, must be positive.
	JWTTTL time.Duration
	// DBMaxConns caps the pgx pool size. DB_MAX_CONNS, default 10.
	DBMaxConns int32
}

// LoadConfig reads configuration via getenv (normally os.Getenv; injected
// for tests). It fails when DATABASE_URL or JWT_SECRET is missing, when
// JWT_TTL is not a positive duration, or when DB_MAX_CONNS is not a
// positive integer.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:        getenv("PORT"),
		DatabaseURL: getenv("DATABASE_URL"),
		JWTSecret:   getenv("JWT_SECRET"),
		JWTTTL:      defaultJWTTTL,
		DBMaxConns:  defaultDBMaxConns,
	}
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if raw := getenv("JWT_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("JWT_TTL must be a positive duration, got %q", raw)
		}
		cfg.JWTTTL = ttl
	}
	if raw := getenv("DB_MAX_CONNS"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("DB_MAX_CONNS must be a positive integer, got %q", raw)
		}
		cfg.DBMaxConns = int32(n)
	}
	return cfg, nil
}
