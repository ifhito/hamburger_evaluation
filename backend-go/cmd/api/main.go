// Command api is the composition root: it loads config, builds the single
// pgx pool, wires the HTTP handlers, and runs the server with resource
// guardrails and graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// main stays thin: signal handling, config, then the testable run().
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := infra.LoadConfig(os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := run(ctx, cfg, nil); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// run builds the pool and handlers, then serves until ctx is canceled
// (SIGTERM/SIGINT in production). The pool is created exactly once here
// and closed after the server has shut down. ready, if non-nil, is called
// with the bound address once the listener is accepting (tests use it with
// PORT=0 for an ephemeral port).
func run(ctx context.Context, cfg infra.Config, ready func(addr string)) error {
	pool, err := infra.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	jwtCodec := infra.NewJWTCodec(cfg.JWTSecret, cfg.JWTTTL)
	userRepo := repository.NewUserRepository(pool)
	auth := usecase.NewAuth(
		userRepo,
		infra.BcryptPasswordHasher{},
		jwtCodec,
		jwtCodec,
	)

	// Review photo storage (S10): S3-compatible in s3 mode, else local
	// disk, which the API also serves itself under GET /photos/ via
	// handler.PhotoFileServer (the dir resolves relative to the working
	// dir; the container mounts . as /app). photoFiles stays nil in s3
	// mode.
	var photos usecase.PhotoStorage
	var photoFiles http.Handler
	if cfg.PhotoStorage == "s3" {
		photos = storage.NewS3(cfg.PhotoS3Endpoint, cfg.PhotoS3Bucket,
			cfg.PhotoS3AccessKeyID, cfg.PhotoS3SecretAccessKey, cfg.PhotoPublicBaseURL)
	} else {
		photos = storage.NewDisk(cfg.PhotoDiskDir, cfg.PhotoPublicBaseURL)
		photoFiles = handler.PhotoFileServer(cfg.PhotoDiskDir)
	}

	shops := usecase.NewShops(repository.NewShopRepository(pool))
	reviews := usecase.NewReviews(repository.NewReviewRepository(pool), photos)
	users := usecase.NewUsers(userRepo, infra.BcryptPasswordHasher{})

	return serve(ctx, cfg.Port, handler.NewRouter(pool, auth, shops, reviews, users, photoFiles), ready)
}

// serve runs an http.Server with explicit timeouts (never bare
// ListenAndServe) until ctx is canceled, then shuts down gracefully so
// in-flight requests complete, and returns nil on a clean shutdown.
func serve(ctx context.Context, port string, h http.Handler, ready func(addr string)) error {
	ln, err := net.Listen("tcp", net.JoinHostPort("", port))
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", port, err)
	}

	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	if ready != nil {
		ready(ln.Addr().String())
	}

	select {
	case err := <-errCh:
		// Serve failed before shutdown was requested.
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
