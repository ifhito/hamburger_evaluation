package infra

import (
	"strings"
	"testing"
	"time"
)

// mailEnv は、確認メールの必須の設定を満たす環境変数である。既存のテストは、
// これを足して、メール以外の設定を検証する。
var mailEnv = map[string]string{
	"SMTP_HOST":    "smtp.example.com",
	"SMTP_PORT":    "587",
	"MAIL_FROM":    "noreply@example.com",
	"APP_BASE_URL": "https://app.example.com",
}

// withMailConfig は、メール以外の設定だけを書いたテストの期待値に、mailEnv が読み込まれた結果を足す。
func withMailConfig(cfg Config) Config {
	cfg.SMTPHost = "smtp.example.com"
	cfg.SMTPPort = 587
	cfg.SMTPSecurity = "starttls"
	cfg.MailFrom = "noreply@example.com"
	cfg.AppBaseURL = "https://app.example.com"
	return cfg
}

// withMailEnv は、env に mailEnv を足したコピーを返す（env にある値が優先される）。
func withMailEnv(env map[string]string) func(string) string {
	return func(key string) string {
		if v, ok := env[key]; ok {
			return v
		}
		return mailEnv[key]
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "デフォルト値が適用される",
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
			name: "明示した値が使われる",
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
			name: "s3 モードで変数をすべて指定すると読み込める",
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
			name: "不正な PHOTO_STORAGE はエラーになる",
			env: map[string]string{
				"DATABASE_URL":  "postgres://localhost/app",
				"JWT_SECRET":    "test-only-secret",
				"PHOTO_STORAGE": "ftp",
			},
			wantErr: true,
		},
		{
			name:    "DATABASE_URL がないとエラーになる",
			env:     map[string]string{"JWT_SECRET": "test-only-secret"},
			wantErr: true,
		},
		{
			name:    "JWT_SECRET がないとエラーになる",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/app"},
			wantErr: true,
		},
		{
			name: "duration でない JWT_TTL はエラーになる",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "soon",
			},
			wantErr: true,
		},
		{
			name: "JWT_TTL が 0 だとエラーになる",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "0s",
			},
			wantErr: true,
		},
		{
			name: "JWT_TTL が負の値だとエラーになる",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"JWT_TTL":      "-1h",
			},
			wantErr: true,
		},
		{
			name: "数値でない DB_MAX_CONNS はエラーになる",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"DB_MAX_CONNS": "lots",
			},
			wantErr: true,
		},
		{
			name: "0 以下の DB_MAX_CONNS はエラーになる",
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
			got, err := LoadConfig(withMailEnv(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadConfig returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig returned error: %v", err)
			}
			if want := withMailConfig(tt.want); got != want {
				t.Fatalf("LoadConfig = %+v, want %+v", got, want)
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
		t.Run("s3 モードで必須の変数が欠けるとエラーになる: "+missing, func(t *testing.T) {
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
			if _, err := LoadConfig(withMailEnv(env)); err == nil {
				t.Fatalf("LoadConfig returned nil error, want error for missing %s", missing)
			}
		})
	}
}

// baseEnv は、メール以外の必須の設定である。
var baseEnv = map[string]string{
	"DATABASE_URL": "postgres://localhost/app",
	"JWT_SECRET":   "test-only-secret",
}

// mailLoader は、メール以外の必須の設定と mailEnv に、overrides を重ねて読み込む。値が "" の
// キーは、未設定として扱う（欠けた場合のテストのため）。
func mailLoader(overrides map[string]string) (Config, error) {
	return LoadConfig(func(key string) string {
		if v, ok := overrides[key]; ok {
			return v
		}
		if v, ok := baseEnv[key]; ok {
			return v
		}
		return mailEnv[key]
	})
}

// TestLoadConfigMail は、確認メールの設定を検証する。
func TestLoadConfigMail(t *testing.T) {
	t.Run("既定値: STARTTLS、認証なし", func(t *testing.T) {
		cfg, err := mailLoader(nil)
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if cfg.SMTPSecurity != "starttls" || cfg.SMTPUser != "" || cfg.SMTPPassword != "" {
			t.Errorf("既定値が違う: %+v", cfg)
		}
	})

	t.Run("明示した値が使われ、APP_BASE_URL の末尾の / は取り除かれる", func(t *testing.T) {
		cfg, err := mailLoader(map[string]string{
			"SMTP_PORT":     "465",
			"SMTP_SECURITY": "tls",
			"SMTP_USER":     "resend",
			"SMTP_PASSWORD": "test-only-api-key",
			"MAIL_FROM":     "Hamburger <noreply@example.com>",
			"APP_BASE_URL":  "http://localhost:5173/",
		})
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if cfg.SMTPPort != 465 || cfg.SMTPSecurity != "tls" || cfg.SMTPUser != "resend" || cfg.SMTPPassword != "test-only-api-key" ||
			cfg.MailFrom != "Hamburger <noreply@example.com>" || cfg.AppBaseURL != "http://localhost:5173" {
			t.Errorf("読み込んだ値が違う: %+v", cfg)
		}
	})

	t.Run("認証情報がなければ、暗号化しない接続(none)でよい", func(t *testing.T) {
		if _, err := mailLoader(map[string]string{"SMTP_SECURITY": "none"}); err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
	})

	failures := []struct {
		name      string
		overrides map[string]string
	}{
		{"SMTP_HOST が欠けている", map[string]string{"SMTP_HOST": ""}},
		{"SMTP_PORT が欠けている", map[string]string{"SMTP_PORT": ""}},
		{"SMTP_PORT が整数でない", map[string]string{"SMTP_PORT": "smtp"}},
		{"SMTP_PORT が範囲外", map[string]string{"SMTP_PORT": "70000"}},
		{"SMTP_SECURITY が未知の値", map[string]string{"SMTP_SECURITY": "ssl"}},
		{"認証情報を設定したまま SMTP_SECURITY=none にする", map[string]string{"SMTP_SECURITY": "none", "SMTP_USER": "u", "SMTP_PASSWORD": "p"}},
		{"SMTP_USER だけがある", map[string]string{"SMTP_USER": "u"}},
		{"SMTP_PASSWORD だけがある", map[string]string{"SMTP_PASSWORD": "p"}},
		{"MAIL_FROM が欠けている", map[string]string{"MAIL_FROM": ""}},
		{"MAIL_FROM がメールアドレスでない", map[string]string{"MAIL_FROM": "not-an-address"}},
		{"APP_BASE_URL が欠けている", map[string]string{"APP_BASE_URL": ""}},
		{"APP_BASE_URL が http(s) でない", map[string]string{"APP_BASE_URL": "ftp://app.example.com"}},
		{"APP_BASE_URL にクエリがある", map[string]string{"APP_BASE_URL": "https://app.example.com/?a=b"}},
	}
	for _, tt := range failures {
		t.Run("起動時に失敗する: "+tt.name, func(t *testing.T) {
			if _, err := mailLoader(tt.overrides); err == nil {
				t.Fatal("LoadConfig returned nil error, want error")
			}
		})
	}

	t.Run("エラーメッセージに SMTP_PASSWORD の値を含めない", func(t *testing.T) {
		_, err := mailLoader(map[string]string{"SMTP_SECURITY": "none", "SMTP_USER": "u", "SMTP_PASSWORD": "super-secret-value"})
		if err == nil || strings.Contains(err.Error(), "super-secret-value") {
			t.Fatalf("error = %v, want error without the password value", err)
		}
	})
}
