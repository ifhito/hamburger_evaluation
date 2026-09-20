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
				Port:               "8080",
				DatabaseURL:        "postgres://localhost/app",
				JWTSecret:          "test-only-secret",
				JWTTTL:             24 * time.Hour,
				DBMaxConns:         10,
				PhotoStorage:       "disk",
				PhotoDiskDir:       "storage/photos",
				PhotoPublicBaseURL: "/photos",
			},
		},
		{
			name: "explicit values",
			env: map[string]string{
				"PORT":                  "9090",
				"DATABASE_URL":          "postgres://localhost/app",
				"JWT_SECRET":            "test-only-secret",
				"JWT_TTL":               "1h30m",
				"DB_MAX_CONNS":          "4",
				"PHOTO_STORAGE":         "disk",
				"PHOTO_DISK_DIR":        "/var/photos",
				"PHOTO_PUBLIC_BASE_URL": "https://cdn.example.com/photos",
			},
			want: Config{
				Port:               "9090",
				DatabaseURL:        "postgres://localhost/app",
				JWTSecret:          "test-only-secret",
				JWTTTL:             90 * time.Minute,
				DBMaxConns:         4,
				PhotoStorage:       "disk",
				PhotoDiskDir:       "/var/photos",
				PhotoPublicBaseURL: "https://cdn.example.com/photos",
			},
		},
		{
			name: "s3 mode with all variables",
			env: map[string]string{
				"DATABASE_URL":               "postgres://localhost/app",
				"JWT_SECRET":                 "test-only-secret",
				"PHOTO_STORAGE":              "s3",
				"PHOTO_S3_ENDPOINT":          "https://acct.r2.cloudflarestorage.com",
				"PHOTO_S3_BUCKET":            "photos",
				"PHOTO_S3_ACCESS_KEY_ID":     "test-only-key-id",
				"PHOTO_S3_SECRET_ACCESS_KEY": "test-only-secret-key",
				"PHOTO_PUBLIC_BASE_URL":      "https://photos.example.com",
			},
			want: Config{
				Port:                   "8080",
				DatabaseURL:            "postgres://localhost/app",
				JWTSecret:              "test-only-secret",
				JWTTTL:                 24 * time.Hour,
				DBMaxConns:             10,
				PhotoStorage:           "s3",
				PhotoPublicBaseURL:     "https://photos.example.com",
				PhotoS3Endpoint:        "https://acct.r2.cloudflarestorage.com",
				PhotoS3Bucket:          "photos",
				PhotoS3AccessKeyID:     "test-only-key-id",
				PhotoS3SecretAccessKey: "test-only-secret-key",
			},
		},
		{
			name: "invalid PHOTO_STORAGE fails",
			env: map[string]string{
				"DATABASE_URL":  "postgres://localhost/app",
				"JWT_SECRET":    "test-only-secret",
				"PHOTO_STORAGE": "ftp",
			},
			wantErr: true,
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

// TestLoadConfigS3MissingVars は、s3 モードで必須の変数のうちどれか 1 つが
// 欠けている場合に fail-loud することを検証する。
func TestLoadConfigS3MissingVars(t *testing.T) {
	required := []string{
		"PHOTO_S3_ENDPOINT",
		"PHOTO_S3_BUCKET",
		"PHOTO_S3_ACCESS_KEY_ID",
		"PHOTO_S3_SECRET_ACCESS_KEY",
		"PHOTO_PUBLIC_BASE_URL",
	}
	for _, missing := range required {
		t.Run("missing "+missing, func(t *testing.T) {
			env := map[string]string{
				"DATABASE_URL":               "postgres://localhost/app",
				"JWT_SECRET":                 "test-only-secret",
				"PHOTO_STORAGE":              "s3",
				"PHOTO_S3_ENDPOINT":          "https://acct.r2.cloudflarestorage.com",
				"PHOTO_S3_BUCKET":            "photos",
				"PHOTO_S3_ACCESS_KEY_ID":     "test-only-key-id",
				"PHOTO_S3_SECRET_ACCESS_KEY": "test-only-secret-key",
				"PHOTO_PUBLIC_BASE_URL":      "https://photos.example.com",
			}
			delete(env, missing)
			if _, err := LoadConfig(func(key string) string { return env[key] }); err == nil {
				t.Fatalf("LoadConfig returned nil error, want error for missing %s", missing)
			}
		})
	}
}
