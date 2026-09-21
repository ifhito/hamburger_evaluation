package infra

import (
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort          = "8080"
	defaultDBMaxConns    = int32(10)
	defaultJWTTTL        = 24 * time.Hour
	defaultPhotoDiskDir  = "storage/photos"
	defaultPhotoBaseURL  = "/photos"
	photoStorageDiskMode = "disk"
	photoStorageS3Mode   = "s3"

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
}

// LoadConfig は getenv を通して設定を読み込む（通常は os.Getenv で、
// テストのために注入される）。DATABASE_URL または JWT_SECRET がない場合、
// JWT_TTL が正の duration でない場合、DB_MAX_CONNS が正の整数でない場合、
// PHOTO_STORAGE が "disk" でも "s3" でもない場合、s3 モードで必須の
// 変数のいずれかが欠けている場合、または確認メールの設定（SMTP_*、MAIL_FROM、
// APP_BASE_URL）が欠けている・不正な場合に失敗する。
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
	return cfg, nil
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
