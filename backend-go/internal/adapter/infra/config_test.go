package infra

import (
	"bytes"
	"log/slog"
	"reflect"
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

// withStatsWorkerDefaults は、期待値に、統計のワーカーの既定の設定を足す(それ以外を指定しないテスト用)。
func withStatsWorkerDefaults(cfg Config) Config {
	cfg.StatsWorkerInterval = time.Second
	cfg.StatsWorkerBatch = 20
	cfg.StatsWorkerMaxAttempts = 8
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
			if want := withStatsWorkerDefaults(withMailConfig(tt.want)); !reflect.DeepEqual(got, want) {
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

func TestLoadConfigStatsWorker(t *testing.T) {
	t.Run("設定すると、その値が使われる", func(t *testing.T) {
		cfg, err := mailLoader(map[string]string{
			"STATS_WORKER_INTERVAL":     "250ms",
			"STATS_WORKER_BATCH":        "5",
			"STATS_WORKER_MAX_ATTEMPTS": "3",
		})
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if cfg.StatsWorkerInterval != 250*time.Millisecond || cfg.StatsWorkerBatch != 5 || cfg.StatsWorkerMaxAttempts != 3 {
			t.Errorf("ワーカーの設定が違う: %+v", cfg)
		}
	})

	// 不正な値は、起動を失敗させずに既定の値になる。ただし、黙って戻すと、設定したつもりの値が
	// 効いていないことに気づけないので、どの変数かを警告のログに出す。
	for name, tc := range map[string]struct {
		env  map[string]string
		want func(Config) bool
	}{
		"STATS_WORKER_INTERVAL が数値でない": {map[string]string{"STATS_WORKER_INTERVAL": "soon"}, func(c Config) bool { return c.StatsWorkerInterval == time.Second }},
		"STATS_WORKER_INTERVAL が 0":    {map[string]string{"STATS_WORKER_INTERVAL": "0s"}, func(c Config) bool { return c.StatsWorkerInterval == time.Second }},
		"STATS_WORKER_INTERVAL が負":     {map[string]string{"STATS_WORKER_INTERVAL": "-1s"}, func(c Config) bool { return c.StatsWorkerInterval == time.Second }},
		"STATS_WORKER_BATCH が数値でない":    {map[string]string{"STATS_WORKER_BATCH": "many"}, func(c Config) bool { return c.StatsWorkerBatch == 20 }},
		"STATS_WORKER_BATCH が 0":       {map[string]string{"STATS_WORKER_BATCH": "0"}, func(c Config) bool { return c.StatsWorkerBatch == 20 }},
		"STATS_WORKER_MAX_ATTEMPTS が負": {map[string]string{"STATS_WORKER_MAX_ATTEMPTS": "-2"}, func(c Config) bool { return c.StatsWorkerMaxAttempts == 8 }},
	} {
		t.Run("不正な値は、警告のログを出して既定の値になる: "+name, func(t *testing.T) {
			var logs bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			cfg, err := mailLoader(tc.env)
			if err != nil {
				t.Fatalf("LoadConfig returned error: %v", err)
			}
			if !tc.want(cfg) {
				t.Errorf("既定の値になっていない: %+v", cfg)
			}
			for key := range tc.env {
				if !strings.Contains(logs.String(), key) {
					t.Errorf("警告のログに変数名 %s がない: %s", key, logs.String())
				}
			}
		})
	}
}

func TestLoadConfigOAuth(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	base := func(extra map[string]string) func(string) string {
		env := map[string]string{"DATABASE_URL": "postgres://localhost/app", "JWT_SECRET": "test-only-secret"}
		for k, v := range extra {
			env[k] = v
		}
		return withMailEnv(env)
	}

	t.Run("OAUTH_ISSUER を設定しなければ、認可サーバーは無効で、ほかの OAUTH_* は読まない", func(t *testing.T) {
		cfg, err := LoadConfig(base(map[string]string{"OAUTH_TOKEN_SECRET": "short", "OAUTH_STATIC_CLIENTS": "not json"}))
		if err != nil || cfg.OAuth.Enabled {
			t.Fatalf("cfg.OAuth = %+v, err = %v, want disabled without error", cfg.OAuth, err)
		}
	})

	t.Run("発行者と秘密の鍵だけを設定すると、宛先と許可の画面の URL は既定値になる", func(t *testing.T) {
		cfg, err := LoadConfig(base(map[string]string{"OAUTH_ISSUER": "http://localhost:8080/", "OAUTH_TOKEN_SECRET": secret}))
		if err != nil {
			t.Fatal(err)
		}
		want := OAuthConfig{
			Enabled: true, Issuer: "http://localhost:8080", Resource: "http://localhost:8080/mcp",
			ConsentURL: "https://app.example.com/oauth/authorize", Secret: secret,
		}
		if cfg.OAuth.Enabled != want.Enabled || cfg.OAuth.Issuer != want.Issuer || cfg.OAuth.Resource != want.Resource ||
			cfg.OAuth.ConsentURL != want.ConsentURL || cfg.OAuth.Secret != want.Secret || len(cfg.OAuth.StaticClients) != 0 {
			t.Errorf("OAuth = %+v, want %+v", cfg.OAuth, want)
		}
	})

	t.Run("宛先・許可の画面・固定のアプリを設定すると、その値が使われる", func(t *testing.T) {
		cfg, err := LoadConfig(base(map[string]string{
			"OAUTH_ISSUER": "https://api.example.com", "OAUTH_TOKEN_SECRET": secret,
			"OAUTH_RESOURCE_URL": "https://api.example.com/mcp/v1", "OAUTH_CONSENT_URL": "https://app.example.com/consent",
			"OAUTH_STATIC_CLIENTS": `[{"id":"dev-app","name":"Dev App","redirect_uris":["http://127.0.0.1/callback"]}]`,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.OAuth.Resource != "https://api.example.com/mcp/v1" || cfg.OAuth.ConsentURL != "https://app.example.com/consent" {
			t.Errorf("OAuth = %+v", cfg.OAuth)
		}
		if len(cfg.OAuth.StaticClients) != 1 || cfg.OAuth.StaticClients[0].ID != "dev-app" || cfg.OAuth.StaticClients[0].Name != "Dev App" {
			t.Errorf("StaticClients = %+v", cfg.OAuth.StaticClients)
		}
	})

	failures := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"秘密の鍵がないと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080"}, "OAUTH_TOKEN_SECRET"},
		{"秘密の鍵が 32 文字より短いと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080", "OAUTH_TOKEN_SECRET": "short"}, "OAUTH_TOKEN_SECRET"},
		{"発行者が URL でないと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "localhost:8080", "OAUTH_TOKEN_SECRET": secret}, "OAUTH_ISSUER"},
		{"宛先が URL でないと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080", "OAUTH_TOKEN_SECRET": secret, "OAUTH_RESOURCE_URL": "mcp"}, "OAUTH_RESOURCE_URL"},
		{"許可の画面の URL が不正だと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080", "OAUTH_TOKEN_SECRET": secret, "OAUTH_CONSENT_URL": "ftp://x"}, "OAUTH_CONSENT_URL"},
		{"固定のアプリが JSON でないと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080", "OAUTH_TOKEN_SECRET": secret, "OAUTH_STATIC_CLIENTS": "nope"}, "OAUTH_STATIC_CLIENTS"},
		{"固定のアプリに知らない項目があると、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080", "OAUTH_TOKEN_SECRET": secret, "OAUTH_STATIC_CLIENTS": `[{"id":"a","name":"a","redirect_uris":["https://a.example.com/cb"],"client_secret":"x"}]`}, "OAUTH_STATIC_CLIENTS"},
		{"固定のアプリの戻り先が規則を満たさないと、起動に失敗する", map[string]string{"OAUTH_ISSUER": "http://localhost:8080", "OAUTH_TOKEN_SECRET": secret, "OAUTH_STATIC_CLIENTS": `[{"id":"a","name":"a","redirect_uris":["http://evil.example.com/cb"]}]`}, "OAUTH_STATIC_CLIENTS"},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(base(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want an error naming %s", err, tt.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("エラーに秘密の鍵の値が含まれている: %v", err)
			}
		})
	}
}

func TestLoadConfigMCPAllowedOrigins(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	load := func(extra map[string]string) (Config, error) {
		env := map[string]string{
			"DATABASE_URL": "postgres://localhost/app", "JWT_SECRET": "test-only-secret",
			"OAUTH_ISSUER": "https://api.example.com/", "OAUTH_TOKEN_SECRET": secret,
		}
		for k, v := range extra {
			env[k] = v
		}
		return LoadConfig(withMailEnv(env))
	}

	t.Run("MCP_ALLOWED_ORIGINS を設定しなければ、許可する Origin は、発行者(OAUTH_ISSUER)の Origin だけになる", func(t *testing.T) {
		cfg, err := load(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(cfg.OAuth.MCPAllowedOrigins, ","); got != "https://api.example.com" {
			t.Errorf("MCPAllowedOrigins = %q, want only the issuer's origin", got)
		}
	})

	t.Run("発行者に path があっても、既定は、その path を除いた Origin になる", func(t *testing.T) {
		cfg, err := load(map[string]string{"OAUTH_ISSUER": "http://localhost:8080/api"})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(cfg.OAuth.MCPAllowedOrigins, ","); got != "http://localhost:8080" {
			t.Errorf("MCPAllowedOrigins = %q, want http://localhost:8080", got)
		}
	})

	t.Run("設定すると、既定(発行者の Origin)を置き換え、正規化・重複の除去・空白と末尾のスラッシュの許容をする", func(t *testing.T) {
		cfg, err := load(map[string]string{"MCP_ALLOWED_ORIGINS": " HTTP://Localhost:5173/ , https://app.example.com:443,https://app.example.com,,"})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(cfg.OAuth.MCPAllowedOrigins, ","); got != "http://localhost:5173,https://app.example.com" {
			t.Errorf("MCPAllowedOrigins = %q, want the two normalized origins only (the issuer's origin is replaced)", got)
		}
	})

	t.Run("認可サーバーが無効(OAUTH_ISSUER なし)なら、MCP_ALLOWED_ORIGINS は読まない", func(t *testing.T) {
		cfg, err := load(map[string]string{"OAUTH_ISSUER": "", "MCP_ALLOWED_ORIGINS": "*"})
		if err != nil || cfg.OAuth.Enabled || len(cfg.OAuth.MCPAllowedOrigins) != 0 {
			t.Errorf("cfg.OAuth = %+v, err = %v, want disabled without error", cfg.OAuth, err)
		}
	})

	failures := map[string]string{
		"ワイルドカード":      "*",
		"ワイルドカードを含む一覧": "https://app.example.com,*",
		"null":         "null",
		"scheme がない":   "app.example.com",
		"http(s) 以外":   "ftp://app.example.com",
		"path を含む":     "https://app.example.com/app",
		"query を含む":    "https://app.example.com?x=1",
		"利用者情報を含む":     "https://user@app.example.com",
		"ポートが範囲外":      "https://app.example.com:99999",
		"区切りだけで、中身がない": " , ,",
	}
	for name, value := range failures {
		t.Run("不正な値("+name+")があると、起動に失敗し、エラーには変数名だけを含める", func(t *testing.T) {
			_, err := load(map[string]string{"MCP_ALLOWED_ORIGINS": value})
			if err == nil || !strings.Contains(err.Error(), "MCP_ALLOWED_ORIGINS") {
				t.Fatalf("err = %v, want an error naming MCP_ALLOWED_ORIGINS", err)
			}
			if strings.Contains(strings.ToLower(err.Error()), "app.example.com") {
				t.Errorf("エラーに値が含まれている: %v", err)
			}
		})
	}
}
