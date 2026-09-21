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
// lastOffset は最後の呼び出しの引数である（handler が usecase に渡した値と、
// 呼ばれなかったことの検証用）。
type shopStoreFake struct {
	shops                 []domain.ShopDetail // Reviews は未設定。下の reviews 経由で提供される
	reviews               map[int64][]domain.ShopReview
	err                   error
	listCalls             int
	lastLimit, lastOffset int32
}

var (
	_ usecase.ShopQuery     = (*shopStoreFake)(nil)
	_ domain.ShopRepository = (*shopStoreFake)(nil)
)

func (f *shopStoreFake) ListShops(_ context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error) {
	f.listCalls++
	f.lastLimit, f.lastOffset = limit, offset
	if f.err != nil {
		return nil, f.err
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
	return out[lo:hi], nil
}

func (f *shopStoreFake) GetShopWithCreator(_ context.Context, id int64) (domain.ShopDetail, error) {
	if f.err != nil {
		return domain.ShopDetail{}, f.err
	}
	for _, d := range f.shops {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.ShopDetail{}, domain.ErrShopNotFound
}

func (f *shopStoreFake) ListShopReviews(_ context.Context, shopID int64) ([]domain.ShopReview, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.reviews[shopID], nil
}

// newShopsRouter は、auth kit と与えられた shop の fake で router を配線し、
// 通常のユーザーと admin 用に発行した Bearer ヘッダーを返す。
func newShopsRouter(t *testing.T, repo *shopStoreFake) (router http.Handler, aliceAuth, adminAuth string, aliceID int64) {
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
	return handler.NewRouter(okPinger, auth, unusedSignups(), usecase.NewShops(repo, domain.NewShops(repo)),
			usecase.NewReviews(reviewRepo, domain.NewReviews(reviewRepo), storage.NewDisk(t.TempDir(), "/photos")),
			usecase.NewUsers(users, domain.NewUsers(users), hasherFake{}), nil),
		"Bearer " + aliceToken, "Bearer " + adminToken, alice.ID
}

func shopPtr[T any](v T) *T { return &v }

// seedShops は、active な shop 1 件、（creatorID が作成した）pending な shop
// 1 件、rejected な shop 1 件を持つ fake を返す。
func seedShops(creatorID int64) *shopStoreFake {
	return &shopStoreFake{
		shops: []domain.ShopDetail{
			{Shop: domain.Shop{ID: 1, Name: "Active Diner", Status: domain.ShopStatusActive}},
			{
				Shop:    domain.Shop{ID: 2, Name: "Alice Pending", Status: domain.ShopStatusPending, CreatorID: shopPtr(creatorID)},
				Creator: &domain.UserRef{ID: creatorID, Username: "alice"},
			},
			{Shop: domain.Shop{ID: 3, Name: "Rejected Grill", Status: domain.ShopStatusRejected, CreatorID: shopPtr(creatorID + 100)}},
		},
		reviews: map[int64][]domain.ShopReview{},
	}
}

// TestListShops は HTTP レベルで AC1–AC3 を扱う：見える集合は OptionalAuth の
// viewer に依存し、body はトップレベルの snake_case の配列で name 順に並ぶ。
func TestListShops(t *testing.T) {
	repo := seedShops(1)
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name       string
		authHeader string
		wantBody   string
	}{
		{
			name:     "AC1 匿名は active な shop だけが見える",
			wantBody: `[{"id":1,"name":"Active Diner","status":"active"}]`,
		},
		{
			name:       "AC2 creator は自分の pending な shop も status 付きで見える",
			authHeader: aliceAuth,
			wantBody:   `[{"id":1,"name":"Active Diner","status":"active"},{"id":2,"name":"Alice Pending","status":"pending"}]`,
		},
		{
			name:       "AC3 admin はすべての status の shop が見える",
			authHeader: adminAuth,
			wantBody:   `[{"id":1,"name":"Active Diner","status":"active"},{"id":2,"name":"Alice Pending","status":"pending"},{"id":3,"name":"Rejected Grill","status":"rejected"}]`,
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
	repo := seedShops(1)
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
			wantBody: `[{"id":1,"name":"Active Diner","status":"active"}]`,
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
			wantBody:   `[{"id":2,"name":"Alice Pending","status":"pending"}]`,
		},
		{
			name:     "範囲外の page と per_page はデフォルト値に fallback する",
			query:    "?page=0&per_page=0",
			wantBody: `[{"id":1,"name":"Active Diner","status":"active"}]`,
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
	repo := seedShops(1)
	repo.reviews[1] = []domain.ShopReview{
		{
			ID:        9,
			Rating:    4,
			Comment:   shopPtr("Tasty"),
			CreatedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
			User:      &domain.UserRef{ID: 3, Username: "bob"},
			Burger: &domain.ShopReviewBurger{
				ID: 5, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8,
			},
		},
		{
			ID:        8,
			Rating:    2,
			CreatedAt: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC),
			User:      &domain.UserRef{ID: 3, Username: "bob"},
			Burger:    &domain.ShopReviewBurger{ID: 6, Name: "Plain"},
		},
	}
	router, _, _, _ := newShopsRouter(t, repo)

	rec := do(router, http.MethodGet, "/shops/1", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := `{"id":1,"name":"Active Diner","status":"active","moderation_note":null,"creator":null,"reviews":[` +
		`{"id":9,"rating":4,"comment":"Tasty","created_at":"2024-05-01T12:00:00Z","photo_url":null,"user":{"id":3,"username":"bob"},` +
		`"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}},` +
		`{"id":8,"rating":2,"comment":null,"created_at":"2024-04-01T12:00:00Z","photo_url":null,"user":{"id":3,"username":"bob"},` +
		`"burger":{"id":6,"name":"Plain","average_rating":0,"review_count":0,"weighted_score":0,"confidence":0}}]}`
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// TestGetShopVisibility は AC4 と AC6 を扱う：pending な shop は匿名の
// viewer には 404 だが creator と admin には開かれており、未知の id と
// 数値でない id は同一の body で 404 になり、失敗は 500 になる。
func TestGetShopVisibility(t *testing.T) {
	repo := seedShops(1)
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)
	const notFoundBody = `{"error":"Shop not found"}`

	tests := []struct {
		name       string
		path       string
		authHeader string
		wantStatus int
	}{
		{name: "AC4 匿名の viewer が pending な shop を開くと 404 になる", path: "/shops/2", wantStatus: http.StatusNotFound},
		{name: "AC4 creator が自分の pending な shop を開くと 200 になる", path: "/shops/2", authHeader: aliceAuth, wantStatus: http.StatusOK},
		{name: "AC4 admin が pending な shop を開くと 200 になる", path: "/shops/2", authHeader: adminAuth, wantStatus: http.StatusOK},
		{name: "creator 以外が rejected な shop を開くと 404 になる", path: "/shops/3", authHeader: aliceAuth, wantStatus: http.StatusNotFound},
		{name: "AC6 未知の id は 404 になる", path: "/shops/999", wantStatus: http.StatusNotFound},
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
		rec := do(failRouter, http.MethodGet, "/shops/1", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}
