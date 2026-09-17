package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
)

const testTimeout = 5 * time.Second

// TestServeGracefulShutdown covers AC4: the server starts on an ephemeral
// port, a request is in flight when the context is canceled (standing in
// for SIGTERM), the in-flight request still completes, and serve returns
// nil without leaking the serve goroutine.
func TestServeGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inFlight := make(chan struct{})
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight) // signal the request has reached the handler
		<-release       // hold it in flight across the shutdown trigger
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	addrCh := make(chan string, 1)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- serve(ctx, "0", h, func(addr string) { addrCh <- addr })
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case err := <-serveErr:
		t.Fatalf("serve exited before ready: %v", err)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for server to start")
	}

	type result struct {
		status int
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		resCh <- result{status: resp.StatusCode}
	}()

	select {
	case <-inFlight:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for request to be in flight")
	}

	cancel()                           // stands in for SIGTERM via signal.NotifyContext
	time.Sleep(100 * time.Millisecond) // let Shutdown begin while the request is held
	close(release)

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("in-flight request failed during shutdown: %v", res.err)
		}
		if res.status != http.StatusOK {
			t.Fatalf("in-flight request status = %d, want %d", res.status, http.StatusOK)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for in-flight request to complete")
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("serve returned %v, want nil after graceful shutdown", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for serve to return (goroutine leak)")
	}
}

// TestRunShutsDownCleanly covers AC4 at the run() level: run wires config,
// pool, and router, serves /up on an ephemeral port (503 here — the pool
// points at an unreachable address and pgxpool connects lazily), and
// returns nil when the context is canceled.
func TestRunShutsDownCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := infra.Config{
		Port: "0",
		// Deliberately unreachable; credentials are dummies for parsing only.
		DatabaseURL: "postgres://user:pass@127.0.0.1:1/hamburger_test?sslmode=disable",
		DBMaxConns:  1,
	}

	addrCh := make(chan string, 1)
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, cfg, func(addr string) { addrCh <- addr })
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case err := <-runErr:
		t.Fatalf("run exited before ready: %v", err)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for run to start")
	}

	resp, err := http.Get("http://" + addr + "/up")
	if err != nil {
		t.Fatalf("GET /up failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /up status = %d, want %d (unreachable db)", resp.StatusCode, http.StatusServiceUnavailable)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned %v, want nil after shutdown", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for run to return")
	}
}

// TestServeListenError asserts serve fails fast with an error when the
// port cannot be bound, instead of hanging.
func TestServeListenError(t *testing.T) {
	err := serve(context.Background(), "not-a-port", http.NotFoundHandler(), nil)
	if err == nil {
		t.Fatal("serve returned nil, want listen error")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected context error: %v", err)
	}
}
