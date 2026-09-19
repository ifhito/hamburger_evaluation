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
	defaultPort          = "8080"
	defaultDBMaxConns    = int32(10)
	defaultJWTTTL        = 24 * time.Hour
	defaultPhotoDiskDir  = "storage/photos"
	defaultPhotoBaseURL  = "/photos"
	photoStorageDiskMode = "disk"
	photoStorageS3Mode   = "s3"
)

// Config is the process configuration loaded from the environment.
type Config struct {
	// Port is the TCP port the HTTP server listens on. PORT, default 8080.
	Port string
	// DatabaseURL is the PostgreSQL connection string. DATABASE_URL, required.
	DatabaseURL string
	// JWTSecret is the HMAC secret for JWT auth. JWT_SECRET, required.
	// For tokens issued by the Rails backend to remain valid, it must
	// equal the Rails secret_key_base.
	// Its value must never be hardcoded or logged.
	JWTSecret string
	// JWTTTL is the lifetime of issued JWTs. JWT_TTL (a Go duration such
	// as "24h"), default 24h, must be positive.
	JWTTTL time.Duration
	// DBMaxConns caps the pgx pool size. DB_MAX_CONNS, default 10.
	DBMaxConns int32
	// PhotoStorage selects the review-photo backend. PHOTO_STORAGE,
	// "disk" (default) or "s3"; any other value is a boot error.
	PhotoStorage string
	// PhotoDiskDir is the root directory for disk-stored photos.
	// PHOTO_DISK_DIR, default "storage/photos"; disk mode only.
	PhotoDiskDir string
	// PhotoPublicBaseURL prefixes public photo URLs.
	// PHOTO_PUBLIC_BASE_URL, default "/photos" in disk mode, required in
	// s3 mode (e.g. an R2 public bucket domain or CDN).
	PhotoPublicBaseURL string
	// PhotoS3Endpoint is the S3 API endpoint (e.g. the Cloudflare R2 S3
	// endpoint). PHOTO_S3_ENDPOINT, required in s3 mode.
	PhotoS3Endpoint string
	// PhotoS3Bucket is the bucket name. PHOTO_S3_BUCKET, required in s3
	// mode.
	PhotoS3Bucket string
	// PhotoS3AccessKeyID is the static access key id.
	// PHOTO_S3_ACCESS_KEY_ID, required in s3 mode.
	// Its value must never be hardcoded or logged.
	PhotoS3AccessKeyID string
	// PhotoS3SecretAccessKey is the static secret access key.
	// PHOTO_S3_SECRET_ACCESS_KEY, required in s3 mode.
	// Its value must never be hardcoded or logged.
	PhotoS3SecretAccessKey string
}

// LoadConfig reads configuration via getenv (normally os.Getenv; injected
// for tests). It fails when DATABASE_URL or JWT_SECRET is missing, when
// JWT_TTL is not a positive duration, when DB_MAX_CONNS is not a positive
// integer, when PHOTO_STORAGE is neither "disk" nor "s3", or when s3 mode
// lacks any of its required variables.
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
		// Fail loudly on the first missing var; error messages name the
		// variable only, never its value.
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
