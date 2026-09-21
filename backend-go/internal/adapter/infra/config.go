package infra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

const (
	defaultPort          = "8080"
	defaultDBMaxConns    = int32(10)
	defaultJWTTTL        = 24 * time.Hour
	defaultPhotoDiskDir  = "storage/photos"
	defaultPhotoBaseURL  = "/photos"
	photoStorageDiskMode = "disk"
	photoStorageS3Mode   = "s3"

	// 統計の再計算のワーカーの既定値。
	defaultStatsWorkerInterval    = time.Second
	defaultStatsWorkerBatch       = 20
	defaultStatsWorkerMaxAttempts = 8

	// SMTP の接続の保護の方式（SMTP_SECURITY）。
	smtpSecurityStartTLS = "starttls"
	smtpSecurityTLS      = "tls"
	smtpSecurityNone     = "none"
)

// Config は環境変数から読み込まれるプロセスの設定である。
type Config struct {
	// Port は HTTP サーバーが listen する TCP ポートである。PORT、
	// デフォルトは 8080。
	Port string
	// DatabaseURL は PostgreSQL の接続文字列である。DATABASE_URL、必須。
	DatabaseURL string
	// JWTSecret は JWT 認証に使う HMAC の secret である。JWT_SECRET、必須。
	// Rails バックエンドが発行したトークンを有効なまま保つには、Rails の
	// secret_key_base と一致していなければならない。
	// その値は決してハードコードしてはならず、ログにも出力してはならない。
	JWTSecret string
	// JWTTTL は発行される JWT の有効期間である。JWT_TTL（"24h" のような
	// Go の duration）、デフォルトは 24h、正の値でなければならない。
	JWTTTL time.Duration
	// DBMaxConns は pgx pool のサイズの上限である。DB_MAX_CONNS、
	// デフォルトは 10。
	DBMaxConns int32
	// PhotoStorage は review の写真の backend を選択する。PHOTO_STORAGE、
	// "disk"（デフォルト）または "s3"。それ以外の値は起動時のエラーになる。
	PhotoStorage string
	// PhotoDiskDir は disk に保存される写真のルートディレクトリである。
	// PHOTO_DISK_DIR、デフォルトは "storage/photos"。disk モードのみ。
	PhotoDiskDir string
	// PhotoPublicBaseURL は公開される写真の URL の接頭辞である。
	// PHOTO_PUBLIC_BASE_URL、disk モードではデフォルトが "/photos"、
	// s3 モードでは必須（例：R2 の公開 bucket のドメインや CDN）。
	PhotoPublicBaseURL string
	// PhotoS3Endpoint は S3 API の endpoint である（例：Cloudflare R2 の
	// S3 endpoint）。PHOTO_S3_ENDPOINT、s3 モードでは必須。
	PhotoS3Endpoint string
	// PhotoS3Bucket は bucket 名である。PHOTO_S3_BUCKET、s3 モードでは
	// 必須。
	PhotoS3Bucket string
	// PhotoS3AccessKeyID は静的な access key id である。
	// PHOTO_S3_ACCESS_KEY_ID、s3 モードでは必須。
	// その値は決してハードコードしてはならず、ログにも出力してはならない。
	PhotoS3AccessKeyID string
	// PhotoS3SecretAccessKey は静的な secret access key である。
	// PHOTO_S3_SECRET_ACCESS_KEY、s3 モードでは必須。
	// その値は決してハードコードしてはならず、ログにも出力してはならない。
	PhotoS3SecretAccessKey string
	// SMTPHost は確認メールを送る SMTP サーバーのホストである。SMTP_HOST、必須。
	SMTPHost string
	// SMTPPort は SMTP サーバーのポートである。SMTP_PORT、必須（1〜65535）。
	// 暗黙の TLS は 465、STARTTLS は 587 が一般的である。
	SMTPPort int
	// SMTPUser は SMTP の認証のユーザー名である。SMTP_USER、認証が不要な環境では省略できる。
	SMTPUser string
	// SMTPPassword は SMTP の認証のパスワード（Resend では API キー）である。
	// SMTP_PASSWORD、認証が不要な環境では省略できる。SMTPUser と対で設定する。
	// その値は決してハードコードしてはならず、ログにも出力してはならない。
	SMTPPassword string
	// SMTPSecurity は SMTP の接続の保護の方式である。SMTP_SECURITY、"starttls"（デフォルト）、
	// "tls"（暗黙の TLS）、"none"（開発用。認証情報を設定したまま "none" にすると、
	// 平文で認証情報を送ってしまうので起動時にエラーになる）。
	SMTPSecurity string
	// MailFrom は確認メールの送信元である。MAIL_FROM、必須。Resend では検証済みのドメインの
	// アドレスにする。
	MailFrom string
	// AppBaseURL は確認リンクの生成元（frontend の URL）である。APP_BASE_URL、必須。
	// 末尾の "/" は取り除かれる。
	AppBaseURL string
	// StatsWorkerInterval は、統計の再計算のワーカーが、依頼を取りに行く間隔である。
	// STATS_WORKER_INTERVAL（"1s" のような Go の duration）、既定は 1s。正の値でなければ既定になる。
	StatsWorkerInterval time.Duration
	// StatsWorkerBatch は、ワーカーが 1 回に取り出す依頼の上限の件数である。STATS_WORKER_BATCH、
	// 既定は 20。正の整数でなければ既定になる。
	StatsWorkerBatch int
	// StatsWorkerMaxAttempts は、1 つの依頼を、失敗しながら再試行する上限の回数である。
	// STATS_WORKER_MAX_ATTEMPTS、既定は 8。正の整数でなければ既定になる。
	StatsWorkerMaxAttempts int
	// OAuth は、OAuth の認可サーバー(AI アプリがログインと許可だけでつなぐための仕組み)の設定である。
	// OAUTH_ISSUER を設定したときだけ有効になり、設定しなければ、認可サーバーの窓口は登録されない。
	OAuth OAuthConfig
	// Google は、Google のアカウントでのサインインの設定である。GOOGLE_CLIENT_ID を設定したときだけ有効になり、
	// 設定しなければ、/auth/google/* は登録されない(404)。
	Google GoogleConfig
}

// GoogleConfig は、Google のアカウントでのサインインの設定である。
type GoogleConfig struct {
	// Enabled は、Google でのサインインを有効にするかである。GOOGLE_CLIENT_ID が設定されているときに true になる。
	Enabled bool
	// ClientID は、Google Cloud Console で作った OAuth クライアントの ID である。GOOGLE_CLIENT_ID。
	ClientID string
	// ClientSecret は、そのクライアントの秘密の鍵である。GOOGLE_CLIENT_SECRET、有効なときは必須。その値は
	// 決してハードコードしてはならず、ログ・エラーの文言にも出力してはならない。
	ClientSecret string
	// RedirectURL は、Google が認可のあとに利用者を戻す URL(この API の /auth/google/callback の公開 URL)である。
	// GOOGLE_REDIRECT_URL、有効なときは必須。Google Cloud Console に登録した「承認済みのリダイレクト URI」と
	// 完全に一致しなければならない。
	RedirectURL string
	// Issuer は、OpenID Connect の提供元の URL である。GOOGLE_OIDC_ISSUER、既定は https://accounts.google.com。
	// テストや隔離した確認で、代役の提供元に向けるためにだけある。https か、ループバック(localhost・127.0.0.1・[::1])の
	// http だけを許す。本番では設定しない。
	Issuer string
}

// defaultGoogleIssuer は、Google の OpenID Connect の提供元の URL である。
const defaultGoogleIssuer = "https://accounts.google.com"

// OAuthConfig は、OAuth の認可サーバーの設定である。
type OAuthConfig struct {
	// Enabled は、認可サーバーを有効にするかである。OAUTH_ISSUER が設定されているときに true になる。
	Enabled bool
	// Issuer は、この API の公開 URL(発行者)である。OAUTH_ISSUER(例: "http://localhost:8080")。
	// 末尾の "/" は取り除かれる。
	Issuer string
	// Resource は、発行するトークンの宛先(トークンを使えるサーバーの URL)である。OAUTH_RESOURCE_URL、
	// 既定は Issuer + "/mcp"。
	Resource string
	// ConsentURL は、利用者に許可を尋ねる画面(frontend)の URL である。OAUTH_CONSENT_URL、
	// 既定は APP_BASE_URL + "/oauth/authorize"。
	ConsentURL string
	// Secret は、トークンの署名に使う秘密の鍵である。OAUTH_TOKEN_SECRET、有効なときは必須で、
	// 32 文字以上でなければならない。その値は決してハードコードしてはならず、ログにも出力してはならない。
	Secret string
	// StaticClients は、固定で登録するアプリである。OAUTH_STATIC_CLIENTS(JSON の配列。
	// [{"id":"…","name":"…","redirect_uris":["…"]}])、省略できる。自分で説明を公開できないアプリや、
	// ローカルでの確認用に使う。
	StaticClients []domain.OAuthClient
	// MCPAllowedOrigins は、リモートの MCP サーバー(/mcp)が受け付ける Origin(ブラウザが要求に付ける
	// 要求元。DNS の付け替え攻撃への対策として、Origin がある要求は、この一覧にあるものだけを通す)である。
	// MCP_ALLOWED_ORIGINS(カンマ区切りの "scheme://host[:port]")、既定は OAUTH_ISSUER の Origin だけ。
	// 設定すると、既定を置き換える。値は domain.NormalizeOrigin で正規化される。ワイルドカードと null は
	// 使えない。Origin のない要求(ブラウザ以外のクライアント)は、この一覧に関係なく通る。
	MCPAllowedOrigins []string
}

// oauthSecretMinLength は、OAUTH_TOKEN_SECRET の最小の文字数である(署名の鍵として 32 バイト以上が必要)。
const oauthSecretMinLength = 32

// LoadConfig は getenv を通して設定を読み込む（通常は os.Getenv で、
// テストのために注入される）。DATABASE_URL または JWT_SECRET がない場合、
// JWT_TTL が正の duration でない場合、DB_MAX_CONNS が正の整数でない場合、
// PHOTO_STORAGE が "disk" でも "s3" でもない場合、s3 モードで必須の
// 変数のいずれかが欠けている場合、または確認メールの設定（SMTP_*、MAIL_FROM、
// APP_BASE_URL）が欠けている・不正な場合に失敗する。統計のワーカーの設定（STATS_WORKER_*）は、
// 不正でも失敗せず、警告のログを出して既定の値になる。
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
	cfg.PhotoStorage = getenv("PHOTO_STORAGE")
	if cfg.PhotoStorage == "" {
		cfg.PhotoStorage = photoStorageDiskMode
	}
	cfg.PhotoPublicBaseURL = getenv("PHOTO_PUBLIC_BASE_URL")
	switch cfg.PhotoStorage {
	case photoStorageDiskMode:
		cfg.PhotoDiskDir = getenv("PHOTO_DISK_DIR")
		if cfg.PhotoDiskDir == "" {
			cfg.PhotoDiskDir = defaultPhotoDiskDir
		}
		if cfg.PhotoPublicBaseURL == "" {
			cfg.PhotoPublicBaseURL = defaultPhotoBaseURL
		}
	case photoStorageS3Mode:
		cfg.PhotoS3Endpoint = getenv("PHOTO_S3_ENDPOINT")
		cfg.PhotoS3Bucket = getenv("PHOTO_S3_BUCKET")
		cfg.PhotoS3AccessKeyID = getenv("PHOTO_S3_ACCESS_KEY_ID")
		cfg.PhotoS3SecretAccessKey = getenv("PHOTO_S3_SECRET_ACCESS_KEY")
		// 必須変数のいずれかが空なら fail-loud する。map の走査順は不定なので、
		// 複数が欠けている場合にどの変数名が報告されるかは決まらない（エラーに
		// 含めるのは 1 つだけ）。エラーメッセージには変数名だけを含め、その値は
		// 決して含めない。
		for name, value := range map[string]string{
			"PHOTO_S3_ENDPOINT":          cfg.PhotoS3Endpoint,
			"PHOTO_S3_BUCKET":            cfg.PhotoS3Bucket,
			"PHOTO_S3_ACCESS_KEY_ID":     cfg.PhotoS3AccessKeyID,
			"PHOTO_S3_SECRET_ACCESS_KEY": cfg.PhotoS3SecretAccessKey,
			"PHOTO_PUBLIC_BASE_URL":      cfg.PhotoPublicBaseURL,
		} {
			if value == "" {
				return Config{}, fmt.Errorf("%s is required when PHOTO_STORAGE=s3", name)
			}
		}
	default:
		return Config{}, fmt.Errorf("PHOTO_STORAGE must be %q or %q, got %q", photoStorageDiskMode, photoStorageS3Mode, cfg.PhotoStorage)
	}
	if err := loadMailConfig(getenv, &cfg); err != nil {
		return Config{}, err
	}
	loadStatsWorkerConfig(getenv, &cfg)
	if err := loadOAuthConfig(getenv, &cfg); err != nil {
		return Config{}, err
	}
	if err := loadGoogleConfig(getenv, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadOAuthConfig は、OAuth の認可サーバーの設定を読み込んで検証する。OAUTH_ISSUER がなければ無効で、
// ほかの OAUTH_* は読まない。有効なのに設定が足りない・不正なときは fail-loud する。エラー
// メッセージには変数名だけを含め、その値（特に OAUTH_TOKEN_SECRET）は決して含めない。
func loadOAuthConfig(getenv func(string) string, cfg *Config) error {
	issuer := strings.TrimRight(getenv("OAUTH_ISSUER"), "/")
	if issuer == "" {
		return nil
	}
	if err := requireHTTPURL("OAUTH_ISSUER", issuer); err != nil {
		return err
	}
	oc := OAuthConfig{Enabled: true, Issuer: issuer, Secret: getenv("OAUTH_TOKEN_SECRET")}
	if len(oc.Secret) < oauthSecretMinLength {
		return fmt.Errorf("OAUTH_TOKEN_SECRET is required and must be at least %d characters when OAUTH_ISSUER is set", oauthSecretMinLength)
	}
	oc.Resource = getenv("OAUTH_RESOURCE_URL")
	if oc.Resource == "" {
		oc.Resource = issuer + "/mcp"
	}
	if err := requireHTTPURL("OAUTH_RESOURCE_URL", oc.Resource); err != nil {
		return err
	}
	oc.ConsentURL = getenv("OAUTH_CONSENT_URL")
	if oc.ConsentURL == "" {
		oc.ConsentURL = cfg.AppBaseURL + "/oauth/authorize"
	}
	if err := requireHTTPURL("OAUTH_CONSENT_URL", oc.ConsentURL); err != nil {
		return err
	}
	if raw := strings.TrimSpace(getenv("OAUTH_STATIC_CLIENTS")); raw != "" {
		var entries []struct {
			ID           string   `json:"id"`
			Name         string   `json:"name"`
			RedirectURIs []string `json:"redirect_uris"`
		}
		dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&entries); err != nil {
			return fmt.Errorf("OAUTH_STATIC_CLIENTS must be a JSON array of {id, name, redirect_uris}")
		}
		for _, e := range entries {
			c := domain.OAuthClient{ID: e.ID, Name: e.Name, RedirectURIs: e.RedirectURIs}
			if err := c.Validate(); err != nil {
				return fmt.Errorf("OAUTH_STATIC_CLIENTS has an invalid client: %w", err)
			}
			oc.StaticClients = append(oc.StaticClients, c)
		}
	}
	origins, err := loadMCPAllowedOrigins(getenv("MCP_ALLOWED_ORIGINS"), issuer)
	if err != nil {
		return err
	}
	oc.MCPAllowedOrigins = origins
	cfg.OAuth = oc
	return nil
}

// loadMCPAllowedOrigins は、MCP_ALLOWED_ORIGINS(raw)を、正規化した Origin の一覧にする。空なら、issuer の
// Origin だけの一覧(既定)である。不正な値(ワイルドカード・null・path を含むもの・空の一覧など)は、起動を
// 失敗させる(変数名だけを含め、値は含めない)。区切りの前後の空白と、値の末尾の "/" 1 つは、許す。
func loadMCPAllowedOrigins(raw, issuer string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		u, err := url.Parse(issuer)
		if err != nil {
			return nil, fmt.Errorf("OAUTH_ISSUER must be an http(s) URL")
		}
		origin, err := domain.NormalizeOrigin(u.Scheme + "://" + u.Host)
		if err != nil {
			return nil, fmt.Errorf("OAUTH_ISSUER must be an http(s) URL with a host")
		}
		return []string{origin}, nil
	}
	var origins []string
	seen := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSuffix(strings.TrimSpace(entry), "/")
		if entry == "" {
			continue
		}
		origin, err := domain.NormalizeOrigin(entry)
		if err != nil {
			return nil, fmt.Errorf("MCP_ALLOWED_ORIGINS must be a comma-separated list of origins (scheme://host[:port]); wildcards, null, paths and other schemes are not allowed")
		}
		if !seen[origin] {
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("MCP_ALLOWED_ORIGINS must list at least one origin (or be left unset to allow only the OAUTH_ISSUER origin)")
	}
	return origins, nil
}

// requireHTTPURL は、name の値 raw が、クエリと断片のない http(s) の URL であることを確かめる。
func requireHTTPURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must be an http(s) URL without query or fragment", name)
	}
	return nil
}

// loadStatsWorkerConfig は、統計の再計算のワーカーの設定を読み込む。統計の更新が少し遅れても、
// アプリ全体を止めるほどのことではないので、設定されていて不正(数値でない・0 以下)な値は、
// 起動を失敗させずに、警告のログを出して既定の値にする(値そのものは秘密ではないのでログに含める)。
func loadStatsWorkerConfig(getenv func(string) string, cfg *Config) {
	cfg.StatsWorkerInterval = defaultStatsWorkerInterval
	if raw := getenv("STATS_WORKER_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			cfg.StatsWorkerInterval = d
		} else {
			slog.Warn("invalid stats worker setting, using the default", "name", "STATS_WORKER_INTERVAL", "value", raw, "default", defaultStatsWorkerInterval)
		}
	}
	cfg.StatsWorkerBatch = positiveIntEnv(getenv, "STATS_WORKER_BATCH", defaultStatsWorkerBatch)
	cfg.StatsWorkerMaxAttempts = positiveIntEnv(getenv, "STATS_WORKER_MAX_ATTEMPTS", defaultStatsWorkerMaxAttempts)
}

// positiveIntEnv は、環境変数 name を正の整数として読む。未設定なら fallback を返し、
// 設定されていて正の整数でなければ、警告のログを出して fallback を返す。
func positiveIntEnv(getenv func(string) string, name string, fallback int) int {
	raw := getenv(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		slog.Warn("invalid stats worker setting, using the default", "name", name, "value", raw, "default", fallback)
		return fallback
	}
	return n
}

// loadMailConfig は、確認メールの設定を読み込んで検証し、cfg に書き込む。エラー
// メッセージには変数名だけを含め、その値（特に SMTP_PASSWORD）は決して含めない。
func loadMailConfig(getenv func(string) string, cfg *Config) error {
	cfg.SMTPHost = getenv("SMTP_HOST")
	cfg.SMTPUser = getenv("SMTP_USER")
	cfg.SMTPPassword = getenv("SMTP_PASSWORD")
	cfg.SMTPSecurity = getenv("SMTP_SECURITY")
	cfg.MailFrom = getenv("MAIL_FROM")
	cfg.AppBaseURL = getenv("APP_BASE_URL")

	if cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP_HOST is required")
	}
	rawPort := getenv("SMTP_PORT")
	if rawPort == "" {
		return fmt.Errorf("SMTP_PORT is required")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("SMTP_PORT must be an integer between 1 and 65535, got %q", rawPort)
	}
	cfg.SMTPPort = port

	if cfg.SMTPSecurity == "" {
		cfg.SMTPSecurity = smtpSecurityStartTLS
	}
	switch cfg.SMTPSecurity {
	case smtpSecurityStartTLS, smtpSecurityTLS, smtpSecurityNone:
	default:
		return fmt.Errorf("SMTP_SECURITY must be %q, %q or %q, got %q", smtpSecurityStartTLS, smtpSecurityTLS, smtpSecurityNone, cfg.SMTPSecurity)
	}
	// 認証情報は、ユーザー名とパスワードを対で設定する。暗号化しない接続では送らない
	// （平文で認証情報が流れるのを、起動時に防ぐ）。
	if (cfg.SMTPUser == "") != (cfg.SMTPPassword == "") {
		return fmt.Errorf("SMTP_USER and SMTP_PASSWORD must be set together")
	}
	if cfg.SMTPUser != "" && cfg.SMTPSecurity == smtpSecurityNone {
		return fmt.Errorf("SMTP_SECURITY=none cannot be used with SMTP_USER and SMTP_PASSWORD (credentials would be sent in plain text)")
	}

	if cfg.MailFrom == "" {
		return fmt.Errorf("MAIL_FROM is required")
	}
	if _, err := mail.ParseAddress(cfg.MailFrom); err != nil {
		return fmt.Errorf("MAIL_FROM must be a valid email address")
	}
	if cfg.AppBaseURL == "" {
		return fmt.Errorf("APP_BASE_URL is required")
	}
	base, err := url.Parse(cfg.AppBaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.RawQuery != "" || base.Fragment != "" {
		return fmt.Errorf("APP_BASE_URL must be an http(s) URL without query or fragment, got %q", cfg.AppBaseURL)
	}
	cfg.AppBaseURL = strings.TrimRight(cfg.AppBaseURL, "/")
	return nil
}

// googleRedirectPathSuffix は、GOOGLE_REDIRECT_URL の path の末尾である(この API の /auth/google/callback。公開の
// path には、前に接頭辞が付いてもよい)。handler が、cookie の Path を、ここから決める。
const googleRedirectPathSuffix = "/auth/google/callback"

// loadGoogleConfig は、Google のアカウントでのサインインの設定を読み込んで検証する。GOOGLE_CLIENT_ID がなければ
// 無効で、ほかの GOOGLE_* は読まない。有効なのに設定が足りない・不正なときは fail-loud する。エラーメッセージには
// 変数名だけを含め、その値(特に GOOGLE_CLIENT_SECRET)は決して含めない。
func loadGoogleConfig(getenv func(string) string, cfg *Config) error {
	clientID := strings.TrimSpace(getenv("GOOGLE_CLIENT_ID"))
	if clientID == "" {
		return nil
	}
	gc := GoogleConfig{
		Enabled:      true,
		ClientID:     clientID,
		ClientSecret: getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  getenv("GOOGLE_REDIRECT_URL"),
		Issuer:       strings.TrimRight(getenv("GOOGLE_OIDC_ISSUER"), "/"),
	}
	if gc.ClientSecret == "" {
		return fmt.Errorf("GOOGLE_CLIENT_SECRET is required when GOOGLE_CLIENT_ID is set")
	}
	if gc.RedirectURL == "" {
		return fmt.Errorf("GOOGLE_REDIRECT_URL is required when GOOGLE_CLIENT_ID is set")
	}
	// 認可コード・state・手続きの cookie が流れる戻り先と、1 回限りのコード(JWT に交換できる)が流れる
	// アプリの URL は、https か、開発用のループバックの http だけを許す(外部への平文の通信を防ぐ)。
	if err := requireHTTPSOrLoopback("GOOGLE_REDIRECT_URL", gc.RedirectURL); err != nil {
		return err
	}
	// 手続きの cookie の Path と、結果との交換の cookie の Path は、戻り先の path から決まる。末尾が違うと、cookie が
	// 届かず、手続きが失敗するのに、気づけないので、起動のときに断る(値はエラーに含めない)。
	if u, _ := url.Parse(gc.RedirectURL); !strings.HasSuffix(u.Path, googleRedirectPathSuffix) {
		return fmt.Errorf("GOOGLE_REDIRECT_URL path must end with %s (a public prefix such as /api may come before it)", googleRedirectPathSuffix)
	}
	if err := requireHTTPSOrLoopback("APP_BASE_URL", cfg.AppBaseURL); err != nil {
		return fmt.Errorf("%w (required when GOOGLE_CLIENT_ID is set)", err)
	}
	if gc.Issuer == "" {
		gc.Issuer = defaultGoogleIssuer
	} else if err := requireHTTPSOrLoopback("GOOGLE_OIDC_ISSUER", gc.Issuer); err != nil {
		return err
	}
	cfg.Google = gc
	return nil
}

// GoogleWarnings は、設定は有効(LoadConfig を通る)でも、実際には Google でのサインインと結び付けが失敗しやすい組み合わせを、
// 起動時のログに出す文言で返す(起動は止めない)。画面は、交換と結び付けの開始を、画面と同じオリジンの /api/… へ
// 送る。GOOGLE_REDIRECT_URL が、APP_BASE_URL(画面)と別のオリジン(たとえば API に直接 http://localhost:8080/…)だと、
// 戻りで設定する「交換の cookie」が、画面からの要求に届かず、毎回、失敗する(気づけない失敗になる)。
// Google でのサインインが無効なとき・URL を読めないときは、何も返さない。値(URL)だけを出し、秘密は含めない。
func (c Config) GoogleWarnings() []string {
	if !c.Google.Enabled {
		return nil
	}
	redirect, err1 := url.Parse(c.Google.RedirectURL)
	app, err2 := url.Parse(c.AppBaseURL)
	if err1 != nil || err2 != nil || urlOrigin(redirect) == urlOrigin(app) {
		return nil
	}
	return []string{fmt.Sprintf(
		"GOOGLE_REDIRECT_URL (%s) は、APP_BASE_URL (%s) と別のオリジンです。画面から /api/… で呼ぶ交換の要求に cookie が届かず、"+
			"Google でのサインインと結び付けが、毎回失敗します。戻り先は、画面のオリジンを通る形(例: %s/api/auth/google/callback)にし、"+
			"Google Cloud の「承認済みのリダイレクト URI」も同じ値にしてください",
		c.Google.RedirectURL, c.AppBaseURL, strings.TrimRight(c.AppBaseURL, "/"))}
}

// urlOrigin は、URL のオリジン(スキーム + ホスト + ポート。大文字小文字は区別しない)を返す。
func urlOrigin(u *url.URL) string {
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// requireHTTPSOrLoopback は、name の値 raw が、https の URL か、ループバック(localhost・127.0.0.1・[::1])の
// http の URL であることを確かめる。平文の http で、外部の提供元に、認可コードやトークンを送らないため。
func requireHTTPSOrLoopback(name, raw string) error {
	if err := requireHTTPURL(name, raw); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	if u.Scheme == "https" {
		return nil
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return nil
	}
	return fmt.Errorf("%s must be an https URL (http is allowed only for localhost, 127.0.0.1 or [::1])", name)
}
