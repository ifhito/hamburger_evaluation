// Package infra holds infrastructure concerns: environment configuration
// and database pool construction. Later stories add JWT and password
// hashing here.
package infra

import (
	"fmt"
	"strconv"
)

const (
	defaultPort       = "8080"
	defaultDBMaxConns = int32(10)
)

// Config is the process configuration loaded from the environment.
type Config struct {
	// Port is the TCP port the HTTP server listens on. PORT, default 8080.
	Port string
	// DatabaseURL is the PostgreSQL connection string. DATABASE_URL, required.
	DatabaseURL string
	// JWTSecret is the HMAC secret for JWT auth. JWT_SECRET. It is read
	// here for later stories; in S1 it may be empty (no auth endpoints yet)
	// and its value must never be hardcoded or logged.
	JWTSecret string
	// DBMaxConns caps the pgx pool size. DB_MAX_CONNS, default 10.
	DBMaxConns int32
}

// LoadConfig reads configuration via getenv (normally os.Getenv; injected
// for tests). It fails when DATABASE_URL is missing or DB_MAX_CONNS is not
// a positive integer.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:        getenv("PORT"),
		DatabaseURL: getenv("DATABASE_URL"),
		JWTSecret:   getenv("JWT_SECRET"),
		DBMaxConns:  defaultDBMaxConns,
	}
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
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
