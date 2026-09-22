// Command api は composition root である。config を読み込み、唯一の
// pgx pool を構築し、HTTP handler を配線し、resource guardrail と
// graceful shutdown を備えた server を実行する。
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/googleauth"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/oauthserver"
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
	logConfigWarnings(cfg)
	if err := run(ctx, cfg, nil); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// logConfigWarnings は、設定は有効でも、実際には失敗しやすい組み合わせ(Google の戻り先が、画面のオリジンを通らない
// など)を、起動時のログに警告として出す(起動は止めない)。手順書は、このメッセージ(と detail の項目)で、原因を探させる。
func logConfigWarnings(cfg infra.Config) {
	for _, w := range cfg.GoogleWarnings() {
		slog.Warn("suspicious google login setting", "detail", w)
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

	shops := usecase.NewShops(query.NewShopQuery(pool), domain.NewShops(repository.NewShopRepository(pool)), photos)
	// 統計の再計算役(BurgerStatsRecalculator・ShopStatsRecalculator)は、その手順を持ち、現在時刻を外から受け取る。
	// 書き込み(レビュー・退会)は、バーガーの統計とショップの集計、両方の再計算の依頼を、同じトランザクションで
	// 登録する(ShopStatsRecalculator のコメントに、ワーカーからではなく書き込みから呼ぶ理由がある)。
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	shopRecalc := usecase.NewShopStatsRecalculator(infra.SystemClock{})
	reviews := usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, shopRecalc, photos)
	users := usecase.NewUsers(userQuery, userWrites, unitOfWork, recalc, shopRecalc, infra.BcryptPasswordHasher{})

	// 統計の再計算は、書き込みの応答を待たせないよう、バックグラウンドのワーカー(goroutine 1 本)が
	// あとから行う。起動した直後に、前回の停止までに溜まっていた依頼を処理する。停止では、サーバーを
	// 止めたあとに、処理中のバッチを終えてから止める(順序は、defer が後ろから実行されることを使い、
	// サーバー停止 → ワーカー停止 → メール送信の停止 → プールを閉じる、になる)。
	// ショップの集計(件数・平均・写真)も、同じ仕組みで、あとから計算する。worker は計算するだけで、依頼の登録は
	// しない(上の reviews・users が登録する)。
	workerCfg := usecase.StatsWorkerConfig{Batch: cfg.StatsWorkerBatch, MaxAttempts: cfg.StatsWorkerMaxAttempts}
	statsWorker := usecase.NewStatsWorker(query.NewBurgerStatsQuery(pool), unitOfWork, recalc, infra.SystemClock{}, workerCfg)
	shopStatsWorker := usecase.NewShopStatsWorker(query.NewShopStatsQuery(pool), unitOfWork, shopRecalc, infra.SystemClock{}, workerCfg)
	statsLoop := infra.StartStatsWorker(infra.NewStatsCycle(statsWorker, shopStatsWorker), cfg.StatsWorkerInterval)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := statsLoop.Stop(ctx); err != nil {
			log.Printf("stats worker: stop: %v", err)
		}
	}()

	// OAuth の認可サーバーとリモートの MCP サーバーは、OAUTH_ISSUER を設定したときだけ有効になる(設定が
	// なければ、窓口は登録されない)。
	var oauth *handler.OAuth
	var mcp handler.MCPEndpoints
	if cfg.OAuth.Enabled {
		// 許可の記録の書き込みは、domain の書き込みオブジェクトを通し、読み取りは query を通す。
		grantWrites := domain.NewOAuthGrants(repository.NewOAuthGrantRepository(pool))
		grantQuery := query.NewOAuthGrantQuery(pool)
		oauthServer, err := oauthserver.New(oauthserver.Config{
			Issuer:        cfg.OAuth.Issuer,
			Resource:      cfg.OAuth.Resource,
			ConsentURL:    cfg.OAuth.ConsentURL,
			Secret:        []byte(cfg.OAuth.Secret),
			StaticClients: cfg.OAuth.StaticClients,
		}, oauthserver.Deps{
			Sessions: uow.NewOAuthTokenSessionStore(pool),
			Grants:   grantQuery,
			Users:    userQuery,
			Fetcher:  oauthserver.NewHTTPMetadataFetcher(),
		})
		if err != nil {
			return fmt.Errorf("oauth server: %w", err)
		}
		oauth = &handler.OAuth{
			Endpoints: oauthServer,
			Consents:  usecase.NewOAuthConsents(oauthServer, grantQuery, grantWrites),
			Apps:      usecase.NewConnectedApps(grantQuery, grantWrites),
		}

		// リモートの MCP サーバー(/mcp)は、認可サーバーのトークンで認証する。ツールの実体は、
		// 上で組み立てた usecase を直接呼ぶ。認可サーバーが無効なら、/mcp も登録されない。
		mcpServer, err := handler.NewMCPServer(
			usecase.NewOAuthAccessTokens(oauthServer, userQuery, cfg.OAuth.Resource),
			shops, reviews, users,
			handler.MCPConfig{Resource: cfg.OAuth.Resource, Issuer: cfg.OAuth.Issuer, AllowedOrigins: cfg.OAuth.MCPAllowedOrigins},
		)
		if err != nil {
			return fmt.Errorf("mcp server: %w", err)
		}
		mcp = mcpServer
	}

	// Google のアカウントでのサインインは、GOOGLE_CLIENT_ID を設定したときだけ有効になる(設定がなければ、
	// 窓口は登録されず、GET /meta の login_providers も空になる)。ユーザーの作成と結び付きの記録は、
	// UnitOfWork の中で 1 つのトランザクションにする。
	var routerOpts []handler.RouterOption
	if cfg.Google.Enabled {
		googleLogins := usecase.NewGoogleLogins(
			googleauth.New(googleauth.Config{
				ClientID:     cfg.Google.ClientID,
				ClientSecret: cfg.Google.ClientSecret,
				RedirectURL:  cfg.Google.RedirectURL,
				Issuer:       cfg.Google.Issuer,
			}),
			query.NewUserIdentityQuery(pool),
			userQuery,
			unitOfWork,
			domain.NewLoginHandoffs(repository.NewLoginHandoffRepository(pool)),
			domain.NewUserIdentities(repository.NewUserIdentityRepository(pool)),
			jwtCodec,
		)
		googleLogin, err := handler.NewGoogleLogin(googleLogins, handler.GoogleLoginConfig{
			AppBaseURL:   cfg.AppBaseURL,
			RedirectURL:  cfg.Google.RedirectURL,
			CookieSecret: cfg.JWTSecret,
		})
		if err != nil {
			return fmt.Errorf("google login: %w", err)
		}
		routerOpts = append(routerOpts, handler.WithGoogleLogin(googleLogin))
	}

	return serve(ctx, cfg.Port, handler.NewRouter(pool, auth, signups, shops, reviews, users, photoFiles, oauth, mcp, routerOpts...), ready)
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
