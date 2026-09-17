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

	return serve(ctx, cfg.Port, handler.NewRouter(pool), ready)
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
