// Package infra は infrastructure に関する関心事（環境変数からの設定、
// database pool の構築、JWT の発行/検証、パスワードのハッシュ化）を保持する。
package infra

import (
	"fmt"
	"strconv"
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
}

// LoadConfig は getenv を通して設定を読み込む（通常は os.Getenv で、
// テストのために注入される）。DATABASE_URL または JWT_SECRET がない場合、
// JWT_TTL が正の duration でない場合、DB_MAX_CONNS が正の整数でない場合、
// PHOTO_STORAGE が "disk" でも "s3" でもない場合、または s3 モードで必須の
// 変数のいずれかが欠けている場合に失敗する。
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
	return cfg, nil
}
