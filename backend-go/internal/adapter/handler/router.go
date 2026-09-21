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

// NewRouter は HTTP handler のツリーを構築する：stdlib の Go 1.22 の
// method パターン mux を、グローバルな body cap の middleware で包んだもの
// である。未知の route は 404、誤った method は 405 を返し、どちらも JSON の
// エラー形式である。photoFiles は nil でない場合（disk への写真保存、S10）、
// GET /photos/ の配下で review の写真を配信する。mux に直接登録しており
// （Go 1.22 の ServeMux は最も限定的なパターンを優先するので、登録順に関わらず
// catch-all の "/" より優先される）、s3 モードでは nil で、写真の URL は
// 代わりに bucket の公開ドメインを指す。
func NewRouter(db Pinger, auth *usecase.Auth, shops *usecase.Shops, reviews *usecase.Reviews, users *usecase.Users, photoFiles http.Handler) http.Handler {
	mux := http.NewServeMux()
	if photoFiles != nil {
		mux.Handle("GET /photos/", http.StripPrefix("/photos/", photoFiles))
	}
	registerRoutes(mux, []route{
		{path: "/up", methods: map[string]http.HandlerFunc{http.MethodGet: handleHealth(db)}},
		{path: "/signup", methods: map[string]http.HandlerFunc{http.MethodPost: handleSignup(auth)}},
		{path: "/login", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogin(auth)}},
		{path: "/logout", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogout}, middleware: RequireAuth(auth)},
		{path: "/me", methods: map[string]http.HandlerFunc{http.MethodGet: handleMe}, middleware: RequireAuth(auth)},
		{path: "/meta", methods: map[string]http.HandlerFunc{http.MethodGet: handleMeta}},
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
	})
	return limitBody(mux)
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
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		})
	}

	// 一致しない path 用の catch-all で、stdlib のプレーンテキストの 404 を
	// 置き換える。
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
}
