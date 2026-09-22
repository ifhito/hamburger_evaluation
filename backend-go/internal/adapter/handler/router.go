package handler

import (
	"net/http"
	"sort"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// route は 1 つの path の handler を HTTP method ごとに宣言し、加えて、その
// path のすべての method に適用される省略可能な middleware を宣言する。
// methodMiddleware は、個々の method についてその path 単位の middleware を
// 上書きするので、1 つの path が、重複する ServeMux パターンなしに、
// 例：GET を OptionalAuth の背後で、POST を RequireAuth の背後で提供できる。
// route をデータにしているので、405 のフォールバック（Allow ヘッダー付き）は
// 手書きではなく導出される。
type route struct {
	path             string
	methods          map[string]http.HandlerFunc
	middleware       func(http.Handler) http.Handler
	methodMiddleware map[string]func(http.Handler) http.Handler
}

// OAuthEndpoints は、OAuth の認可サーバーの HTTP の窓口である(実装は adapter/oauthserver)。
// プロトコルの細部(認可・トークン・取り消しの要求の解釈と応答)は実装が担い、ここは URL と HTTP メソッドを
// 結び付けるだけである。
type OAuthEndpoints interface {
	HandleMetadata(http.ResponseWriter, *http.Request)
	HandleAuthorize(http.ResponseWriter, *http.Request)
	HandleToken(http.ResponseWriter, *http.Request)
	HandleRevoke(http.ResponseWriter, *http.Request)
}

// RouterOption は、NewRouter の省略できる設定である(有効なときだけ渡す機能の窓口)。
type RouterOption func(*routerExtras)

type routerExtras struct {
	google *GoogleLogin
}

// WithGoogleLogin は、Google のアカウントでのサインインの窓口を登録する。渡さなければ(Google での
// サインインが無効なとき)、/auth/google/* と、外部のサービスとの結び付けの API は未登録で、404 になる。
func WithGoogleLogin(g *GoogleLogin) RouterOption {
	return func(e *routerExtras) { e.google = g }
}

// NewRouter は HTTP handler のツリーを構築する：stdlib の Go 1.22 の
// method パターン mux を、グローバルな body cap の middleware で包んだもの
// である。未知の route は 404、誤った method は 405 を返し、どちらも JSON の
// エラー形式である。photoFiles は nil でない場合（disk への写真保存）、
// GET /photos/ の配下で review の写真を配信する。mux に直接登録しており
// （Go 1.22 の ServeMux は最も限定的なパターンを優先するので、登録順に関わらず
// catch-all の "/" より優先される）、s3 モードでは nil で、写真の URL は
// 代わりに bucket の公開ドメインを指す。oauth は nil でない場合（OAuth の認可サーバーが有効な
// とき）、認可サーバーの情報・認可・トークン・取り消しの窓口と、許可の画面・許可したアプリの一覧が使う
// API を登録する。nil なら、これらは未登録で、404 になる。mcp は nil でない場合（リモートの MCP サーバーが
// 有効なとき）、POST /mcp と保護されたリソースの情報の窓口を登録する。nil なら未登録で、404 になる。
func NewRouter(db Pinger, auth *usecase.Auth, signups *usecase.Signups, shops *usecase.Shops, reviews *usecase.Reviews, users *usecase.Users, photoFiles http.Handler, oauth *OAuth, mcp MCPEndpoints, opts ...RouterOption) http.Handler {
	var extras routerExtras
	for _, opt := range opts {
		opt(&extras)
	}
	mux := http.NewServeMux()
	if photoFiles != nil {
		mux.Handle("GET /photos/", http.StripPrefix("/photos/", photoFiles))
	}
	registerRoutes(mux, append(append(append(oauthRoutes(oauth, auth), mcpRoutes(mcp)...), googleRoutes(extras.google, auth)...), []route{
		{path: "/up", methods: map[string]http.HandlerFunc{http.MethodGet: handleHealth(db)}},
		{path: "/signup", methods: map[string]http.HandlerFunc{http.MethodPost: handleSignup(signups)}},
		{path: "/signup/confirm", methods: map[string]http.HandlerFunc{http.MethodPost: handleSignupConfirm(signups)}},
		{path: "/login", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogin(auth)}},
		{path: "/logout", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogout}, middleware: RequireAuth(auth)},
		{path: "/me", methods: map[string]http.HandlerFunc{http.MethodGet: handleMe}, middleware: RequireAuth(auth)},
		{path: "/meta", methods: map[string]http.HandlerFunc{http.MethodGet: handleMeta(extras.google.LoginProviders())}},
		// GET は匿名でも使える（OptionalAuth）ままで、ログインが必要なのは
		// shop の投稿だけであり、そのため method ごとの上書きを行う。
		{
			path: "/shops",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:  handleListShops(shops),
				http.MethodPost: handleCreateShop(shops),
			},
			middleware:       OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{http.MethodPost: RequireAuth(auth)},
		},
		{path: "/shops/{id}", methods: map[string]http.HandlerFunc{http.MethodGet: handleGetShop(shops)}, middleware: OptionalAuth(auth)},
		// review のフィードと詳細は匿名でも使える。ログインが必要なのは
		// 書き込み（POST/PUT/DELETE）だけである。
		{
			path: "/reviews",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:  handleListReviews(reviews),
				http.MethodPost: handleCreateReview(reviews),
			},
			middleware:       OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{http.MethodPost: RequireAuth(auth)},
		},
		{
			path: "/reviews/{id}",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:    handleGetReview(reviews),
				http.MethodPut:    handleUpdateReview(reviews),
				http.MethodDelete: handleDeleteReview(reviews),
			},
			middleware: OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{
				http.MethodPut:    RequireAuth(auth),
				http.MethodDelete: RequireAuth(auth),
			},
		},
		// user の詳細は匿名でも使える（path 単位は OptionalAuth）。ただし email と
		// admin が入るのは本人が閲覧したときだけで、その判断は domain
		// （User.ProfileFor）にある。account の編集と削除にはログインが必要で
		// （PUT/DELETE だけ methodMiddleware で RequireAuth）、本人のみという
		// ルール自体は usecase にある（ErrForbidden）。user の一覧は提供しない
		// ので "/users" は登録せず、catch-all の 404 になる。
		{
			path: "/users/{id}",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:    handleGetUser(users),
				http.MethodPut:    handleUpdateUser(users),
				http.MethodDelete: handleDeleteUser(users),
			},
			middleware: OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{
				http.MethodPut:    RequireAuth(auth),
				http.MethodDelete: RequireAuth(auth),
			},
		},
		// moderation の endpoint：RequireAuth は認証だけを行い、
		// admin かどうかの判断自体は usecase にある（ErrForbidden）。
		{path: "/admin/shops", methods: map[string]http.HandlerFunc{http.MethodGet: handleAdminListShops(shops)}, middleware: RequireAuth(auth)},
		{path: "/admin/shops/{id}", methods: map[string]http.HandlerFunc{http.MethodPut: handleAdminUpdateShop(shops)}, middleware: RequireAuth(auth)},
		{path: "/admin/shops/{id}/approve", methods: map[string]http.HandlerFunc{http.MethodPost: handleApproveShop(shops)}, middleware: RequireAuth(auth)},
		{path: "/admin/shops/{id}/reject", methods: map[string]http.HandlerFunc{http.MethodPost: handleRejectShop(shops)}, middleware: RequireAuth(auth)},
	}...))
	return limitBody(mux)
}

// oauthRoutes は、OAuth の認可サーバーの route を返す。oauth が nil なら、何も返さない。
// 認可の URL は、利用者のブラウザが開く(ログインの確認は、frontend の許可の画面で行う)ので、認証の
// middleware は付けない。トークンと取り消しは、アプリが直接呼ぶ。許可の画面の API と許可したアプリの
// 一覧は、利用者本人の JWT で守る(RequireAuth)。
func oauthRoutes(oauth *OAuth, auth *usecase.Auth) []route {
	if oauth == nil {
		return nil
	}
	return []route{
		{path: "/.well-known/oauth-authorization-server", methods: map[string]http.HandlerFunc{http.MethodGet: oauth.Endpoints.HandleMetadata}},
		{path: "/oauth/authorize", methods: map[string]http.HandlerFunc{http.MethodGet: oauth.Endpoints.HandleAuthorize}},
		{path: "/oauth/token", methods: map[string]http.HandlerFunc{http.MethodPost: oauth.Endpoints.HandleToken}},
		{path: "/oauth/revoke", methods: map[string]http.HandlerFunc{http.MethodPost: oauth.Endpoints.HandleRevoke}},
		{path: "/oauth/authorize/request", methods: map[string]http.HandlerFunc{http.MethodGet: handleOAuthAuthorizeRequest(oauth.Consents)}, middleware: RequireAuth(auth)},
		{path: "/oauth/authorize/decision", methods: map[string]http.HandlerFunc{http.MethodPost: handleOAuthDecision(oauth.Consents)}, middleware: RequireAuth(auth)},
		{path: "/oauth/grants", methods: map[string]http.HandlerFunc{http.MethodGet: handleListOAuthGrants(oauth.Apps)}, middleware: RequireAuth(auth)},
		{path: "/oauth/grants/{id}", methods: map[string]http.HandlerFunc{http.MethodDelete: handleRevokeOAuthGrant(oauth.Apps)}, middleware: RequireAuth(auth)},
	}
}

// googleRoutes は、Google のアカウントでのサインインの route を返す。g が nil なら、何も返さない。
// 認可の画面への移動(GET /auth/google/start。サインイン・新規登録の手続きだけ)と、Google からの戻りは、利用者の
// ブラウザが開くので、認証の middleware は付けない。**結び付ける利用者は、URL やコードでは伝えない**: 結び付けは、
// 利用者本人の JWT で守った POST /me/identities/google/link から始め、結び付ける利用者は、その要求を出したブラウザの
// 手続きの cookie にだけ持たせる(開始の URL やコードを、別のブラウザに開かせても、結び付けられない)。
// 結果との交換(POST /auth/google/exchange)は、「画面へ渡すコード」と、手続きを終えたブラウザの cookie にある
// 結び付けの値の両方が資格なので、認証の middleware は付けない(コードだけでは、交換できない)。
// 結び付けの API(開始・一覧・解除)は、利用者本人の JWT で守る(RequireAuth)。
func googleRoutes(g *GoogleLogin, auth *usecase.Auth) []route {
	if g == nil {
		return nil
	}
	return []route{
		{path: "/auth/google/start", methods: map[string]http.HandlerFunc{http.MethodGet: g.HandleStart}},
		{path: "/auth/google/callback", methods: map[string]http.HandlerFunc{http.MethodGet: g.HandleCallback}},
		{path: "/auth/google/exchange", methods: map[string]http.HandlerFunc{http.MethodPost: g.HandleExchange}},
		{path: "/me/identities", methods: map[string]http.HandlerFunc{http.MethodGet: g.HandleListIdentities}, middleware: RequireAuth(auth)},
		{path: "/me/identities/google/link", methods: map[string]http.HandlerFunc{http.MethodPost: g.HandleLinkStart}, middleware: RequireAuth(auth)},
		{path: "/me/identities/google", methods: map[string]http.HandlerFunc{http.MethodDelete: g.HandleUnlinkGoogle}, middleware: RequireAuth(auth)},
	}
}

// mcpRoutes は、リモートの MCP サーバーの route を返す。mcp が nil なら、何も返さない。POST /mcp の
// 認証(OAuth のアクセストークン)は、ツールごとに必要な範囲が違うため、窓口の中で行う(RequireAuth は
// 付けない)。保護されたリソースの情報は、トークンなしで読める。
func mcpRoutes(mcp MCPEndpoints) []route {
	if mcp == nil {
		return nil
	}
	routes := []route{
		{path: "/mcp", methods: map[string]http.HandlerFunc{http.MethodPost: mcp.HandleMCP}},
		{path: protectedResourceMetadataRoot, methods: map[string]http.HandlerFunc{http.MethodGet: mcp.HandleProtectedResourceMetadata}},
	}
	if p := mcp.ProtectedResourceMetadataPath(); p != protectedResourceMetadataRoot {
		routes = append(routes, route{path: p, methods: map[string]http.HandlerFunc{http.MethodGet: mcp.HandleProtectedResourceMetadata}})
	}
	return routes
}

// registerRoutes は、各 route の "METHOD path" パターン（method ごとの
// middleware が宣言されていればそれで、なければ path 単位の middleware で
// 包む）、宣言された method だけをカンマ区切りでソートした Allow ヘッダー
// 付きで 405 を返す path ごとの method なしのフォールバック、そして JSON の
// 404 の catch-all を登録する。
func registerRoutes(mux *http.ServeMux, routes []route) {
	for _, rt := range routes {
		allowed := make([]string, 0, len(rt.methods))
		for method, h := range rt.methods {
			allowed = append(allowed, method)
			var handler http.Handler = h
			mw := rt.middleware
			if perMethod, ok := rt.methodMiddleware[method]; ok {
				mw = perMethod
			}
			if mw != nil {
				handler = mw(handler)
			}
			mux.Handle(method+" "+rt.path, handler)
		}
		sort.Strings(allowed)
		allow := strings.Join(allowed, ", ")
		mux.HandleFunc(rt.path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", allow)
			writeError(w, r, http.StatusMethodNotAllowed, msgMethodNotAllowed)
		})
	}

	// 一致しない path 用の catch-all で、stdlib のプレーンテキストの 404 を
	// 置き換える。
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusNotFound, msgRouteNotFound)
	})
}
