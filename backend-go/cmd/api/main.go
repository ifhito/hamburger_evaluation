// Command api は composition root である。config を読み込み、唯一の
// pgx pool を構築し、HTTP handler を配線し、resource guardrail と
// graceful shutdown を備えた server を実行する。
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
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// main は薄いままにしておく：signal の処理、config、そしてテスト可能な run()。
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

// run は pool と handler を構築し、ctx が cancel されるまで（本番では
// SIGTERM/SIGINT）serve する。pool はここでちょうど 1 回だけ作成され、
// server の shutdown 後に close される。ready が nil でなければ、listener が
// accept を開始した時点で、bind されたアドレスを渡して呼び出される（テストでは
// ephemeral port を得るために PORT=0 とともに使う）。
func run(ctx context.Context, cfg infra.Config, ready func(addr string)) error {
	pool, err := infra.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	jwtCodec := infra.NewJWTCodec(cfg.JWTSecret, cfg.JWTTTL)
	// 書き込みは、repository を domain の書き込みオブジェクトで包んで usecase に渡す。
	// usecase は repository に依存せず、repository を呼ぶのは domain のコードだけである。
	userQuery := query.NewUserQuery(pool)
	userWrites := domain.NewUsers(repository.NewUserRepository(pool))
	auth := usecase.NewAuth(
		userQuery,
		userWrites,
		infra.BcryptPasswordHasher{},
		jwtCodec,
		jwtCodec,
	)

	// Review 写真の storage（S10）：s3 モードでは S3 互換、それ以外では
	// ローカル disk であり、後者は API 自身も handler.PhotoFileServer を通して
	// GET /photos/ 配下で配信する（dir は作業ディレクトリからの相対で
	// 解決される。container は . を /app としてマウントする）。s3 モードでは
	// photoFiles は nil のままである。
	var photos usecase.PhotoStorage
	var photoFiles http.Handler
	if cfg.PhotoStorage == "s3" {
		photos = storage.NewS3(cfg.PhotoS3Endpoint, cfg.PhotoS3Bucket,
			cfg.PhotoS3AccessKeyID, cfg.PhotoS3SecretAccessKey, cfg.PhotoPublicBaseURL)
	} else {
		photos = storage.NewDisk(cfg.PhotoDiskDir, cfg.PhotoPublicBaseURL)
		photoFiles = handler.PhotoFileServer(cfg.PhotoDiskDir)
	}

	shops := usecase.NewShops(query.NewShopQuery(pool), domain.NewShops(repository.NewShopRepository(pool)))
	// トランザクションをまたぐ手順（review の書き込みと burger の統計の再計算、退会と統計の再計算）は、
	// usecase が UnitOfWork.Do の中で組み立てる。
	unitOfWork := uow.New(pool)
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	reviews := usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, photos)
	users := usecase.NewUsers(userQuery, userWrites, unitOfWork, recalc, infra.BcryptPasswordHasher{})

	return serve(ctx, cfg.Port, handler.NewRouter(pool, auth, shops, reviews, users, photoFiles), ready)
}

// serve は、明示的な timeout を設定した http.Server（素の ListenAndServe は
// 決して使わない）を ctx が cancel されるまで実行し、その後 graceful に
// shutdown して in-flight のリクエストを完了させ、正常に shutdown できたときは
// nil を返す。
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
		// shutdown が要求される前に Serve が失敗した。
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
