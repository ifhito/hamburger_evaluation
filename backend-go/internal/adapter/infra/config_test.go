package infra

import (
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "defaults applied",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
			},
			want: Config{
				Port:        "8080",
				DatabaseURL: "postgres://localhost/app",
				JWTSecret:   "test-only-secret",
				JWTTTL:      24 * time.Hour,
				DBMaxConns:  10,
			},
		},
		{
			name: "explicit values",
			env: map[string]string{
				"PORT":         "9090",
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "1h30m",
				"DB_MAX_CONNS": "4",
			},
			want: Config{
				Port:        "9090",
				DatabaseURL: "postgres://localhost/app",
				JWTSecret:   "test-only-secret",
				JWTTTL:      90 * time.Minute,
				DBMaxConns:  4,
			},
		},
		{
			name:    "missing DATABASE_URL fails",
			env:     map[string]string{"JWT_SECRET": "test-only-secret"},
			wantErr: true,
		},
		{
			name:    "missing JWT_SECRET fails",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/app"},
			wantErr: true,
		},
		{
			name: "non-duration JWT_TTL fails",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "soon",
			},
			wantErr: true,
		},
		{
			name: "zero JWT_TTL fails",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "0s",
			},
			wantErr: true,
		},
		{
			name: "negative JWT_TTL fails",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "-1h",
			},
			wantErr: true,
		},
		{
			name: "non-numeric DB_MAX_CONNS fails",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"DB_MAX_CONNS": "lots",
			},
			wantErr: true,
		},
		{
			name: "non-positive DB_MAX_CONNS fails",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"DB_MAX_CONNS": "0",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadConfig(func(key string) string { return tt.env[key] })
			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadConfig returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("LoadConfig = %+v, want %+v", got, tt.want)
			}
		})
	}
}
