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
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
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
	// HEIC のデコーダ(WASM の読み込みとコンパイル。約 0.3 秒)を、最初の投稿を待たずに、起動と並行して
	// 済ませる(サーバーの起動は待たない)。
	go photo.WarmUp()
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
		infra.BcryptPasswordHasher{},
		jwtCodec,
		jwtCodec,
	)

	// signup はメール確認つきである。メールは非同期に送り（有界のキューと少数の worker）、
	// 送信の失敗・遅延は signup の応答に影響しない。shutdown では、サーバーを止めたあとに
	// キューを送り切ってから閉じる。
	smtpMailer, err := infra.NewSMTPMailer(cfg)
	if err != nil {
		return err
	}
	mailer := infra.NewAsyncMailer(smtpMailer, domain.NewMailDeliveries(repository.NewMailDeliveryRepository(pool)))
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := mailer.Close(ctx); err != nil {
			log.Printf("mailer: close: %v", err)
		}
	}()
	// レビューの保存とバーガーの統計の再計算、ユーザーの退会と統計の再計算、確認メールのリンクを開いたときの
	// ユーザーの作成と確認待ちの削除は、それぞれ、全部成功したときだけ確定し、途中で失敗したら全部取り消す
	// 必要がある。そのため usecase は、UnitOfWork(ここからここまでをまとめて 1 つのトランザクションにする
	// 範囲を、usecase が指定する仕組み)の中でこれらを行う。
	unitOfWork := uow.New(pool)
	signups := usecase.NewSignups(
		userQuery,
		domain.NewSignupVerifications(repository.NewSignupVerificationRepository(pool)),
		unitOfWork,
		infra.BcryptPasswordHasher{},
		mailer,
		jwtCodec,
		usecase.SignupConfig{BaseURL: cfg.AppBaseURL},
	)

	// レビュー写真の保存先：s3 モードでは S3 互換、それ以外では
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
	// 統計の再計算役(BurgerStatsRecalculator)は、その手順を持ち、現在時刻を外から受け取る。
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	reviews := usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, photos)
	users := usecase.NewUsers(userQuery, userWrites, unitOfWork, recalc, infra.BcryptPasswordHasher{})

	return serve(ctx, cfg.Port, handler.NewRouter(pool, auth, signups, shops, reviews, users, photoFiles), ready)
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
