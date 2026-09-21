package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
)

const testTimeout = 5 * time.Second

// TestServeGracefulShutdown は AC4 をカバーする。server が ephemeral port で
// 起動し、context が cancel された（SIGTERM の代わり）時点でリクエストが
// in-flight であり、その in-flight のリクエストがそれでも完了し、serve が
// serve goroutine を leak させずに nil を返す。
func TestServeGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inFlight := make(chan struct{})
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight) // リクエストが handler に到達したことを知らせる
		<-release       // shutdown のトリガーをまたいで in-flight のまま保持する
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

	cancel() // signal.NotifyContext 経由の SIGTERM の代わり

	// Shutdown が始まるまで決定的に待つ：Shutdown はまず listener を close
	// するので、保持していた handler を解放する前に、新しい TCP dial が拒否
	// されるまで poll する。これにより shutdown が始まった時点でリクエストが
	// まだ in-flight であることが保証され、下の assertion が空虚に通ることは
	// ない。
	deadline := time.Now().Add(testTimeout)
	for {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			break // listener が close された：shutdown が進行中である
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			close(release)
			t.Fatal("timed out waiting for shutdown to close the listener")
		}
		time.Sleep(5 * time.Millisecond)
	}
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

// TestRunShutsDownCleanly は run() レベルで AC4 をカバーする。run は config、
// pool、router を配線し、ephemeral port で /up を serve し（ここでは 503。
// pool は到達不能なアドレスを指しており、pgxpool は遅延して接続する）、
// context が cancel されたとき nil を返す。
func TestRunShutsDownCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := infra.Config{
		Port: "0",
		// 意図的に到達不能にしている。認証情報はパース専用のダミーである。
		DatabaseURL: "postgres://user:pass@127.0.0.1:1/hamburger_test?sslmode=disable",
		// JWT_SECRET は config で必須であり、その値はテスト用のダミーである。
		JWTSecret:  "test-secret",
		JWTTTL:     time.Minute,
		DBMaxConns: 1,
		// LoadConfig の disk モードのデフォルトが設定するとおりの写真の
		// storage で、使い捨ての dir を指している。
		PhotoStorage:       "disk",
		PhotoDiskDir:       t.TempDir(),
		PhotoPublicBaseURL: "/photos",
		// 確認メールの設定（S16）。このテストはメールを送らないので、送信先は到達不能なままでよい。
		SMTPHost:     "127.0.0.1",
		SMTPPort:     1,
		SMTPSecurity: "none",
		MailFrom:     "noreply@example.com",
		AppBaseURL:   "http://localhost:5173",
		// 統計のワーカーの設定(LoadConfig の既定と同じ)。データベースには届かないので、
		// 各サイクルは失敗するが、停止のときに、処理中のサイクルを待って終わることを確かめる。
		StatsWorkerInterval:    time.Second,
		StatsWorkerBatch:       20,
		StatsWorkerMaxAttempts: 8,
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

// TestServeListenError は、port を bind できないとき、serve がハングせずに
// エラーで fail-fast することを検証する。
func TestServeListenError(t *testing.T) {
	err := serve(context.Background(), "not-a-port", http.NotFoundHandler(), nil)
	if err == nil {
		t.Fatal("serve returned nil, want listen error")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected context error: %v", err)
	}
}
