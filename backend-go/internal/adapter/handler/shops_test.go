package handler_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// shopStoreFake は in-memory の usecase.ShopQuery かつ domain.ShopRepository
// である。in-memory の fake は共有 DB の代役なので、読み書きで状態を共有する
// よう 1 つの型に保つ（読み書きの分離は、usecase の Query の引数型と domain の
// 書き込みオブジェクトの引数型がコンパイル時に保証する）。可視性は domain の記述子そのもの（vis.CanView）を通して適用
// されるので、そのルールをここで再実装してはいない。keyword のマッチングは
// 単純な case-fold の部分文字列一致である（メタ文字のセマンティクスは
// repository の統合テストが扱う）。err を設定するとすべての操作が失敗する
// （500 の経路）。listCalls は ListShops が呼ばれた回数、lastLimit /
// lastOffset / lastSort は最後の呼び出しの引数である（handler が usecase に渡した値と、
// 呼ばれなかったことの検証用）。
type shopStoreFake struct {
	shops                 []domain.ShopDetail // Reviews は未設定。下の reviews 経由で提供される
	reviews               map[string][]domain.ShopReview
	err                   error
	listCalls             int
	lastLimit, lastOffset int32
	lastSort              usecase.ShopSort
	// summaries は shop の id ごとの、保存された集計(query が返す)。ない shop は「まだ集計されていない」(空の集計)になる。
	summaries map[string]domain.ShopSummary
}

var (
	_ usecase.ShopQuery     = (*shopStoreFake)(nil)
	_ domain.ShopRepository = (*shopStoreFake)(nil)
)

func (f *shopStoreFake) ListShops(_ context.Context, vis domain.ShopVisibility, keyword string, sortOrder usecase.ShopSort, limit, offset int32) ([]domain.ShopListing, bool, error) {
	f.listCalls++
	f.lastLimit, f.lastOffset = limit, offset
	f.lastSort = sortOrder
	if f.err != nil {
		return nil, false, f.err
	}
	var out []domain.Shop
	for _, d := range f.shops {
		if !vis.CanView(d.Shop) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(d.Name), strings.ToLower(keyword)) {
			continue
		}
		out = append(out, d.Shop)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	lo := min(int(offset), len(out))
	hi := min(lo+int(limit), len(out))
	listings := make([]domain.ShopListing, 0, hi-lo)
	for _, shop := range out[lo:hi] {
		listings = append(listings, domain.ShopListing{Shop: shop, Summary: f.summaries[shop.ID]})
	}
	return listings, hi < len(out), nil
}

func (f *shopStoreFake) GetShopWithCreator(_ context.Context, id string) (domain.ShopDetail, error) {
	if f.err != nil {
		return domain.ShopDetail{}, f.err
	}
	for _, d := range f.shops {
		if d.ID == id {
			d.Summary = f.summaries[id]
			return d, nil
		}
	}
	return domain.ShopDetail{}, domain.ErrShopNotFound
}

func (f *shopStoreFake) ListShopReviews(_ context.Context, shopID string) ([]domain.ShopReview, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.reviews[shopID], nil
}

// newShopsRouter は、auth kit と与えられた shop の fake で router を配線し、
// 通常のユーザーと admin 用に発行した Bearer ヘッダーを返す。
func newShopsRouter(t *testing.T, repo *shopStoreFake) (router http.Handler, aliceAuth, adminAuth string, aliceID string) {
	t.Helper()
	users, auth, codec := newAuthKit()
	alice := users.seed("alice", "alice@example.com", "Password123!")
	admin := users.seed("root", "root@example.com", "Password123!")
	users.users[admin.ID].user.Admin = true
	aliceToken, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue alice token: %v", err)
	}
	adminToken, err := codec.Issue(admin.ID)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	reviewRepo := newReviewStoreFake()
	return handler.NewRouter(okPinger, auth, unusedSignups(), shopsUsecase(repo, repo), nil,
			reviewsUsecase(reviewRepo, storage.NewDisk(t.TempDir(), "/photos")),
			usersUsecase(users, hasherFake{}), nil, nil, nil),
		"Bearer " + aliceToken, "Bearer " + adminToken, alice.ID
}

func shopPtr[T any](v T) *T { return &v }

// seedShops は、active な shop 1 件、（creatorID が作成した）pending な shop
// 1 件、rejected な shop 1 件を持つ fake を返す。
func seedShops(creatorID string) *shopStoreFake {
	return &shopStoreFake{
		shops: []domain.ShopDetail{
			{Shop: domain.Shop{ID: uid.N(1), Name: "Active Diner", Status: domain.ShopStatusActive}},
			{
				Shop:    domain.Shop{ID: uid.N(2), Name: "Alice Pending", Status: domain.ShopStatusPending, CreatorID: shopPtr(creatorID)},
				Creator: &domain.UserRef{ID: creatorID, Username: "alice"},
			},
			{Shop: domain.Shop{ID: uid.N(3), Name: "Rejected Grill", Status: domain.ShopStatusRejected, CreatorID: shopPtr(uid.N(99))}},
		},
		reviews: map[string][]domain.ShopReview{},
	}
}

// TestListShops は HTTP レベルで、閲覧者ごとの一覧を扱う：見える集合は OptionalAuth の
// viewer に依存し、body はトップレベルの snake_case の配列で name 順に並ぶ。
func TestListShops(t *testing.T) {
	repo := seedShops(uid.N(1))
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name       string
		authHeader string
		wantBody   string
	}{
		{
			name:     "匿名の閲覧者には、承認済みのショップだけが見える",
			wantBody: `[{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0}]`,
		},
		{
			name:       "作成者には、自分の承認待ちのショップも、状態つきで見える",
			authHeader: aliceAuth,
			wantBody:   `[{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0},{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0}]`,
		},
		{
			name:       "管理者には、すべての状態のショップが見える",
			authHeader: adminAuth,
			wantBody:   `[{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0},{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0},{"id":"` + uid.N(3) + `","name":"Rejected Grill","status":"rejected","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/shops", "", tt.authHeader)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestListShopsParams は HTTP レベルで keyword による絞り込みと pagination の
// fallback を扱う：範囲外の整数はエラーにならずデフォルト値に fallback し、
// 範囲外の page は空配列を返す（整数でない値の 422 は pagination_test.go が扱う）。
func TestListShopsParams(t *testing.T) {
	repo := seedShops(uid.N(1))
	router, _, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name       string
		query      string
		authHeader string
		wantBody   string
	}{
		{
			name:     "keyword は部分文字列で絞り込む",
			query:    "?keyword=diner",
			wantBody: `[{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0}]`,
		},
		{
			name:     "keyword に一致するものがなければ空配列になる",
			query:    "?keyword=nope",
			wantBody: `[]`,
		},
		{
			name:       "per_page=1 page=2 は 2 番目の shop を返す",
			query:      "?per_page=1&page=2",
			authHeader: adminAuth,
			wantBody:   `[{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0}]`,
		},
		{
			name:     "範囲外の page と per_page はデフォルト値に fallback する",
			query:    "?page=0&per_page=0",
			wantBody: `[{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","map_url":null,"closed_at":null,"photo_url":null,"average_rating":null,"review_count":0}]`,
		},
		{
			name:     "データの範囲外の page は空配列になる",
			query:    "?page=99",
			wantBody: `[]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/shops"+tt.query, "", tt.authHeader)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestListShopsSort は、sort クエリパラメータが usecase.Shops.List にそのまま渡ることを
// 確かめる（並び替えの SQL 自体は internal/adapter/query の DB 統合テストが扱う。ここは配線
// だけを確かめる）。sort=newest は usecase.ShopSortNewest になり、省略や未知の値は、すべて
// ゼロ値（既定の店名順）に fallback する（422 にはしない）。
func TestListShopsSort(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  usecase.ShopSort
	}{
		{name: "sort を省略すると既定値(空文字)が渡る", query: "", want: ""},
		{name: "sort=newest は usecase.ShopSortNewest として渡る", query: "?sort=newest", want: usecase.ShopSortNewest},
		{name: "未知の sort の値も、そのまま usecase まで渡る(ShopSortNewest と一致しないので、query 側が既定の店名順として扱う)", query: "?sort=bogus", want: usecase.ShopSort("bogus")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seedShops(uid.N(1))
			router, _, _, _ := newShopsRouter(t, repo)
			rec := do(router, http.MethodGet, "/shops"+tt.query, "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if repo.lastSort != tt.want {
				t.Errorf("lastSort = %q, want %q", repo.lastSort, tt.want)
			}
		})
	}
}

// TestListShopsRepoFailure：repository の失敗は 500 として表面化する。
func TestListShopsRepoFailure(t *testing.T) {
	router, _, _, _ := newShopsRouter(t, &shopStoreFake{err: io.ErrUnexpectedEOF})
	rec := do(router, http.MethodGet, "/shops", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	if got := rec.Body.String(); got != `{"error":"internal server error"}` {
		t.Errorf("body = %q, want the 500 JSON error", got)
	}
}

// TestGetShopDetail は detail の body を厳密に検証する：snake_case の
// フィールド、creator を持たない shop での creator の null、そして user、
// burger、統計を inline で含む review（comment のない review は null、統計の
// ない burger はゼロ）。
func TestGetShopDetail(t *testing.T) {
	repo := seedShops(uid.N(1))
	repo.reviews[uid.N(1)] = []domain.ShopReview{
		{
			ID:        uid.N(9),
			Rating:    4,
			Comment:   shopPtr("Tasty"),
			PhotoKey:  shopPtr("reviews/cheese.jpg"),
			CreatedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
			User:      &domain.UserRef{ID: uid.N(3), Username: "bob"},
			Burger: &domain.ShopReviewBurger{
				ID: uid.N(5), Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8,
			},
		},
		{
			ID:        uid.N(8),
			Rating:    2,
			CreatedAt: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC),
			User:      &domain.UserRef{ID: uid.N(3), Username: "bob"},
			Burger:    &domain.ShopReviewBurger{ID: uid.N(6), Name: "Plain"},
		},
	}
	router, _, _, _ := newShopsRouter(t, repo)

	rec := do(router, http.MethodGet, "/shops/"+uid.N(1), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := `{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","moderation_note":null,"map_url":null,"closed_at":null,"creator":null,"reviews":[` +
		`{"id":"` + uid.N(9) + `","rating":4,"comment":"Tasty","created_at":"2024-05-01T12:00:00Z","visited_at":null,"photo_url":"/photos/reviews/cheese.jpg","user":{"id":"` + uid.N(3) + `","username":"bob"},` +
		`"burger":{"id":"` + uid.N(5) + `","name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}},` +
		`{"id":"` + uid.N(8) + `","rating":2,"comment":null,"created_at":"2024-04-01T12:00:00Z","visited_at":null,"photo_url":null,"user":{"id":"` + uid.N(3) + `","username":"bob"},` +
		`"burger":{"id":"` + uid.N(6) + `","name":"Plain","average_rating":0,"review_count":0,"weighted_score":0,"confidence":0}}],"can_review":false,"photo_url":null,"average_rating":null,"review_count":0}`
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// TestGetShopVisibility は、詳細の見え方を扱う：pending な shop は匿名の
// viewer には 404 だが creator と admin には開かれており、未知の id と
// UUID の正規形でない id は同一の body で 404 になり、失敗は 500 になる。
func TestGetShopVisibility(t *testing.T) {
	repo := seedShops(uid.N(1))
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)
	const notFoundBody = `{"error":"Shop not found"}`

	tests := []struct {
		name       string
		path       string
		authHeader string
		wantStatus int
	}{
		{name: "匿名の閲覧者が審査待ちのショップを開くと 404 になる", path: "/shops/" + uid.N(2), wantStatus: http.StatusNotFound},
		{name: "申請した本人が自分の審査待ちのショップを開くと 200 になる", path: "/shops/" + uid.N(2), authHeader: aliceAuth, wantStatus: http.StatusOK},
		{name: "管理者が審査待ちのショップを開くと 200 になる", path: "/shops/" + uid.N(2), authHeader: adminAuth, wantStatus: http.StatusOK},
		{name: "creator 以外が rejected な shop を開くと 404 になる", path: "/shops/" + uid.N(3), authHeader: aliceAuth, wantStatus: http.StatusNotFound},
		{name: "存在しない id のショップを開くと 404 になる", path: "/shops/" + uid.N(999), wantStatus: http.StatusNotFound},
		{name: "整数の id(1)でショップを開くと、存在しないショップと同じ 404 になる", path: "/shops/1", wantStatus: http.StatusNotFound},
		{name: "UUID ではない文字列の id(abc)でショップを開くと 404 になる", path: "/shops/abc", wantStatus: http.StatusNotFound},
		{name: "大文字の UUID の id でショップを開くと、正規形(小文字)ではないので 404 になる", path: "/shops/" + upperUUID, wantStatus: http.StatusNotFound},
		{name: "非数値の id は同じ 404 になる", path: "/shops/abc", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, tt.path, "", tt.authHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusNotFound {
				if got := rec.Body.String(); got != notFoundBody {
					t.Errorf("body = %q, want %q", got, notFoundBody)
				}
			}
		})
	}

	t.Run("repository の失敗は 500 を返す", func(t *testing.T) {
		failRouter, _, _, _ := newShopsRouter(t, &shopStoreFake{err: fmt.Errorf("db down")})
		rec := do(failRouter, http.MethodGet, "/shops/"+uid.N(1), "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}
