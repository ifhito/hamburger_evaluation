package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeStoredReview は reviewRepoFake の中の review 1 行である。
type fakeStoredReview struct {
	review    domain.Review
	discarded bool
}

// reviewRepoFake は in-memory の usecase.ReviewQuery かつ
// domain.ReviewRepository である。in-memory の fake は共有 DB の代役なので、
// 読み書きで状態を共有するよう 1 つの型に保つ（読み書きの分離は、usecase の Query の
// 引数型と domain のサービスの引数型がコンパイル時に保証する）。active な shop の feed の filter は、seed
// された shops と links から導出される（SQL の EXISTS を再現するもので、SQL
// 自体は repository の統合テストが扱う）。err を設定するとすべての操作が失敗
// する（500 の経路）。listFilters は ListReviews が受け取った filter を呼び出し
// 順に記録する（handler が usecase に渡した値と、呼ばれなかったことの検証用）。
// lastLimit / lastOffset は最後の呼び出しの引数である。
type reviewRepoFake struct {
	shops                 map[int64]domain.Shop
	links                 map[int64][]int64 // shopID -> 紐づく burger の id
	burgers               map[int64]domain.ShopReviewBurger
	usernames             map[int64]string
	seq                   int64
	reviews               map[int64]*fakeStoredReview
	err                   error
	listFilters           []usecase.ReviewListFilter
	lastLimit, lastOffset int32
}

var (
	_ usecase.ReviewQuery     = (*reviewRepoFake)(nil)
	_ domain.ReviewRepository = (*reviewRepoFake)(nil)
)

func newReviewRepoFake() *reviewRepoFake {
	return &reviewRepoFake{
		shops:     map[int64]domain.Shop{},
		links:     map[int64][]int64{},
		burgers:   map[int64]domain.ShopReviewBurger{},
		usernames: map[int64]string{},
		reviews:   map[int64]*fakeStoredReview{},
	}
}

// reviewBaseTime は、決定的な created_at の値の基準となる（n 番目に作成された
// ものは reviewBaseTime + n 分になる）。
var reviewBaseTime = time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

func (f *reviewRepoFake) detailFor(review domain.Review) domain.ReviewDetail {
	burger := f.burgers[review.BurgerID]
	return domain.ReviewDetail{
		Review: review,
		User:   &domain.UserRef{ID: review.AuthorID, Username: f.usernames[review.AuthorID]},
		Burger: &burger,
	}
}

func (f *reviewRepoFake) ListReviews(_ context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
	f.listFilters = append(f.listFilters, filter)
	f.lastLimit, f.lastOffset = limit, offset
	if f.err != nil {
		return nil, f.err
	}
	activeBurgers := map[int64]bool{}
	for shopID, shop := range f.shops {
		if shop.Status != domain.ShopStatusActive {
			continue
		}
		for _, burgerID := range f.links[shopID] {
			activeBurgers[burgerID] = true
		}
	}
	// この filter は SQL の narg の述語を再現する：rating の完全一致、comment
	// に対する大文字小文字を区別しないリテラルな部分文字列一致、
	// shops_burgers の link、author の一致（正確な SQL は repository の
	// 統合テストが扱う）。
	matches := func(review domain.Review) bool {
		if filter.Rating != nil && review.Rating != *filter.Rating {
			return false
		}
		if filter.Keyword != "" && (review.Comment == nil ||
			!strings.Contains(strings.ToLower(*review.Comment), strings.ToLower(filter.Keyword))) {
			return false
		}
		// SQL と同様に、filter の shop 自体が active でなければならない。
		if filter.ShopID != nil && (f.shops[*filter.ShopID].Status != domain.ShopStatusActive ||
			!slices.Contains(f.links[*filter.ShopID], review.BurgerID)) {
			return false
		}
		if filter.UserID != nil && review.AuthorID != *filter.UserID {
			return false
		}
		return true
	}
	var out []domain.Review
	for _, rec := range f.reviews {
		if !rec.discarded && activeBurgers[rec.review.BurgerID] && matches(rec.review) {
			out = append(out, rec.review)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	lo := min(int(offset), len(out))
	hi := min(lo+int(limit), len(out))
	details := make([]domain.ReviewDetail, 0, hi-lo)
	for _, review := range out[lo:hi] {
		details = append(details, f.detailFor(review))
	}
	return details, nil
}

func (f *reviewRepoFake) GetReview(_ context.Context, id int64) (domain.ReviewDetail, error) {
	if f.err != nil {
		return domain.ReviewDetail{}, f.err
	}
	if rec, ok := f.reviews[id]; ok && !rec.discarded {
		return f.detailFor(rec.review), nil
	}
	return domain.ReviewDetail{}, domain.ErrReviewNotFound
}

func (f *reviewRepoFake) GetShop(_ context.Context, id int64) (domain.Shop, error) {
	if f.err != nil {
		return domain.Shop{}, f.err
	}
	if shop, ok := f.shops[id]; ok {
		return shop, nil
	}
	return domain.Shop{}, domain.ErrShopNotFound
}

func (f *reviewRepoFake) GetShopBurger(_ context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
	if f.err != nil {
		return domain.ShopReviewBurger{}, f.err
	}
	if slices.Contains(f.links[shopID], burgerID) {
		return f.burgers[burgerID], nil
	}
	return domain.ShopReviewBurger{}, domain.ErrBurgerNotFound
}

func (f *reviewRepoFake) CreateReview(_ context.Context, review domain.Review) (domain.Review, error) {
	if f.err != nil {
		return domain.Review{}, f.err
	}
	f.seq++
	review.ID = f.seq
	review.CreatedAt = reviewBaseTime.Add(time.Duration(f.seq) * time.Minute)
	f.reviews[review.ID] = &fakeStoredReview{review: review}
	return review, nil
}

func (f *reviewRepoFake) CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
	if f.err != nil {
		return domain.Review{}, domain.ShopReviewBurger{}, f.err
	}
	var burger domain.ShopReviewBurger
	found := false
	for _, burgerID := range f.links[shopID] {
		// id が最小のものが勝つ。SQL の ORDER BY b.id LIMIT 1 を再現している。
		if b := f.burgers[burgerID]; b.Name == burgerName && (!found || b.ID < burger.ID) {
			burger, found = b, true
		}
	}
	if !found {
		var next int64 = 1
		for id := range f.burgers {
			next = max(next, id+1)
		}
		burger = domain.ShopReviewBurger{ID: next, Name: burgerName}
		f.burgers[next] = burger
		f.links[shopID] = append(f.links[shopID], next)
	}
	review.BurgerID = burger.ID
	created, err := f.CreateReview(ctx, review)
	if err != nil {
		return domain.Review{}, domain.ShopReviewBurger{}, err
	}
	return created, burger, nil
}

func (f *reviewRepoFake) UpdateReviewContent(_ context.Context, id int64, rating int, comment string) (domain.Review, error) {
	if f.err != nil {
		return domain.Review{}, f.err
	}
	rec, ok := f.reviews[id]
	if !ok || rec.discarded {
		return domain.Review{}, domain.ErrReviewNotFound
	}
	rec.review.Rating = rating
	c := comment
	rec.review.Comment = &c
	return rec.review, nil
}

func (f *reviewRepoFake) UpdateReviewContentAndPhotoKey(_ context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error) {
	if f.err != nil {
		return domain.Review{}, f.err
	}
	rec, ok := f.reviews[id]
	if !ok || rec.discarded {
		return domain.Review{}, domain.ErrReviewNotFound
	}
	rec.review.Rating = rating
	c := comment
	rec.review.Comment = &c
	rec.review.PhotoKey = photoKey
	return rec.review, nil
}

func (f *reviewRepoFake) DiscardReview(_ context.Context, id int64) error {
	if f.err != nil {
		return f.err
	}
	rec, ok := f.reviews[id]
	if !ok || rec.discarded {
		return domain.ErrReviewNotFound
	}
	rec.discarded = true
	return nil
}

const (
	activeShopID   = int64(1)
	pendingShopID  = int64(2)
	rejectedShopID = int64(3)
	active2ShopID  = int64(4)
	cheeseBurgerID = int64(5)
	plainBurgerID  = int64(6)
)

// seedReviewWorld は、fake に S6 の fixture を入れる：active な shop 2 件
// （activeShopID と active2ShopID）、（creatorID が作成した）pending な shop、
// rejected な shop。そして、Cheese burger（統計あり）は上記 4 件すべての shop に
// 紐づき、Plain burger（統計なし）は pending な shop のみに紐づく。
func seedReviewWorld(creatorID int64) *reviewRepoFake {
	repo := newReviewRepoFake()
	repo.shops[activeShopID] = domain.Shop{ID: activeShopID, Name: "Active Diner", Status: domain.ShopStatusActive}
	repo.shops[pendingShopID] = domain.Shop{ID: pendingShopID, Name: "Alice Pending", Status: domain.ShopStatusPending, CreatorID: shopPtr(creatorID)}
	repo.shops[rejectedShopID] = domain.Shop{ID: rejectedShopID, Name: "Rejected Grill", Status: domain.ShopStatusRejected}
	repo.shops[active2ShopID] = domain.Shop{ID: active2ShopID, Name: "Second Diner", Status: domain.ShopStatusActive}
	repo.burgers[cheeseBurgerID] = domain.ShopReviewBurger{
		ID: cheeseBurgerID, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8,
	}
	repo.burgers[plainBurgerID] = domain.ShopReviewBurger{ID: plainBurgerID, Name: "Plain"}
	// Cheese は active な shop の「両方」（link が重複するケース）と、pending な
	// shop、rejected な shop で提供され、Plain は pending な shop のみで提供される。
	repo.links[activeShopID] = []int64{cheeseBurgerID}
	repo.links[active2ShopID] = []int64{cheeseBurgerID}
	repo.links[pendingShopID] = []int64{cheeseBurgerID, plainBurgerID}
	repo.links[rejectedShopID] = []int64{cheeseBurgerID}
	return repo
}

// newReviewsRouter は、auth kit と与えられた review の fake で router を
// 配線し、alice（id 1）、bob（id 2）、admin（id 3）用の Bearer ヘッダーを
// 返す。fake の usernames の map は、それらの id に揃えられている。
func newReviewsRouter(t *testing.T, repo *reviewRepoFake) (router http.Handler, aliceAuth, bobAuth, adminAuth string) {
	t.Helper()
	router, _, aliceAuth, bobAuth, adminAuth = newPhotoReviewsRouter(t, repo)
	return router, aliceAuth, bobAuth, adminAuth
}

// newPhotoReviewsRouter は newReviewsRouter の本体である：新しい temp dir
// （photoDir。ファイルの assertion 用に返される）を root とする本物の disk
// store が、disk モードで cmd/api が配線するのと同じ handler.PhotoFileServer
// ラッパーを通じて GET /photos/ の配下で配信される。newReviewsRouter は
// これに委譲し、photoDir を捨てるだけである。
func newPhotoReviewsRouter(t *testing.T, repo *reviewRepoFake) (router http.Handler, photoDir, aliceAuth, bobAuth, adminAuth string) {
	t.Helper()
	users, auth, codec := newAuthKit()
	alice := users.seed("alice", "alice@example.com", "Password123!")
	bob := users.seed("bob", "bob@example.com", "Password123!")
	admin := users.seed("root", "root@example.com", "Password123!")
	users.users[admin.ID].user.Admin = true
	repo.usernames[alice.ID] = "alice"
	repo.usernames[bob.ID] = "bob"
	repo.usernames[admin.ID] = "root"
	token := func(id int64) string {
		t.Helper()
		tok, err := codec.Issue(id)
		if err != nil {
			t.Fatalf("issue token for %d: %v", id, err)
		}
		return "Bearer " + tok
	}
	photoDir = t.TempDir()
	shopRepo := &shopRepoFake{}
	router = handler.NewRouter(okPinger, auth, usecase.NewShops(shopRepo, domain.NewShopService(shopRepo)),
		usecase.NewReviews(repo, domain.NewReviewService(repo), storage.NewDisk(photoDir, "/photos")),
		usecase.NewUsers(users, domain.NewUserService(users), hasherFake{}), handler.PhotoFileServer(photoDir))
	return router, photoDir, token(alice.ID), token(bob.ID), token(admin.ID)
}

// TestCreateReview は HTTP レベルで AC1 と AC2 を扱う：active な shop への
// 投稿は正確な payload を伴う 201 を返し、その review は feed と detail に
// 現れる。rejected な shop と他人の pending な shop は 403 を返し、一方で
// creator と admin は pending な shop に投稿できる。
func TestCreateReview(t *testing.T) {
	t.Run("AC1 認証済みユーザーが active な shop に投稿すると 201 を返し、一覧と詳細に現れる", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":"Tasty","shop_id":%d,"burger_id":%d}}`, activeShopID, cheeseBurgerID)
		rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
		}
		want := `{"id":1,"rating":4,"comment":"Tasty","created_at":"2024-06-01T12:01:00Z","photo_url":null,"user":{"id":1,"username":"alice"},` +
			`"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}

		list := do(router, http.MethodGet, "/reviews", "", "")
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, want %d (body %s)", list.Code, http.StatusOK, list.Body)
		}
		if got := list.Body.String(); got != "["+want+"]" {
			t.Errorf("list body = %s, want [%s]", got, want)
		}

		detail := do(router, http.MethodGet, "/reviews/1", "", "")
		if detail.Code != http.StatusOK {
			t.Fatalf("detail status = %d, want %d (body %s)", detail.Code, http.StatusOK, detail.Body)
		}
		if got := detail.Body.String(); got != want {
			t.Errorf("detail body = %s, want %s", got, want)
		}
	})

	t.Run("AC2 review 可否のルール: rejected は全員 403、pending は creator と admin のみ投稿できる", func(t *testing.T) {
		router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, seedReviewWorld(1))
		post := func(auth string, shopID int64) *doResult {
			body := fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_id":%d}}`, shopID, cheeseBurgerID)
			rec := do(router, http.MethodPost, "/reviews", body, auth)
			return &doResult{code: rec.Code, body: rec.Body.String()}
		}
		tests := []struct {
			name     string
			auth     string
			shopID   int64
			wantCode int
		}{
			{name: "pending な shop の creator が rejected な shop に投稿すると 403 になる", auth: aliceAuth, shopID: rejectedShopID, wantCode: http.StatusForbidden},
			{name: "admin が rejected な shop に投稿すると 403 になる", auth: adminAuth, shopID: rejectedShopID, wantCode: http.StatusForbidden},
			{name: "creator が自分の pending な shop に投稿すると 201 になる", auth: aliceAuth, shopID: pendingShopID, wantCode: http.StatusCreated},
			{name: "他のユーザーが pending な shop に投稿すると 403 になる", auth: bobAuth, shopID: pendingShopID, wantCode: http.StatusForbidden},
			{name: "admin が pending な shop に投稿すると 201 になる", auth: adminAuth, shopID: pendingShopID, wantCode: http.StatusCreated},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := post(tt.auth, tt.shopID)
				if got.code != tt.wantCode {
					t.Fatalf("status = %d, want %d (body %s)", got.code, tt.wantCode, got.body)
				}
				if tt.wantCode == http.StatusForbidden && got.body != `{"error":"Forbidden"}` {
					t.Errorf("body = %s, want the Forbidden shape", got.body)
				}
			})
		}
	})

	t.Run("未知の shop と shop に紐づいていない burger はそれぞれの 404 body を返す", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
		rec := do(router, http.MethodPost, "/reviews",
			fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":999,"burger_id":%d}}`, cheeseBurgerID), aliceAuth)
		if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Shop not found"}` {
			t.Errorf("unknown shop = %d %s, want 404 Shop not found", rec.Code, rec.Body)
		}
		// Plain は存在するが、active な shop では提供されていない。
		rec = do(router, http.MethodPost, "/reviews",
			fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_id":%d}}`, activeShopID, plainBurgerID), aliceAuth)
		if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Burger not found"}` {
			t.Errorf("unlinked burger = %d %s, want 404 Burger not found", rec.Code, rec.Body)
		}
	})

	t.Run("burger_name は shop にある同名の burger を再利用する (S6 P3-1)", func(t *testing.T) {
		repo := seedReviewWorld(1)
		router, aliceAuth, _, _ := newReviewsRouter(t, repo)
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":"Tasty","shop_id":%d,"burger_name":"Cheese"}}`, activeShopID)
		rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
		}
		want := `{"id":1,"rating":4,"comment":"Tasty","created_at":"2024-06-01T12:01:00Z","photo_url":null,"user":{"id":1,"username":"alice"},` +
			`"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want the existing Cheese burger %s", got, want)
		}
		if len(repo.burgers) != 2 {
			t.Errorf("burgers = %d, want no new burger for an existing name", len(repo.burgers))
		}
	})

	t.Run("未知の burger_name は burger とその link を作成する (S6 P3-1)", func(t *testing.T) {
		repo := seedReviewWorld(1)
		router, aliceAuth, _, _ := newReviewsRouter(t, repo)
		body := fmt.Sprintf(`{"review":{"rating":5,"comment":"New","shop_id":%d,"burger_name":"Veggie"}}`, activeShopID)
		rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
		}
		// 作成された burger（fake の id 7）の統計はゼロである。
		want := `{"id":1,"rating":5,"comment":"New","created_at":"2024-06-01T12:01:00Z","photo_url":null,"user":{"id":1,"username":"alice"},` +
			`"burger":{"id":7,"name":"Veggie","average_rating":0,"review_count":0,"weighted_score":0,"confidence":0}}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want the created burger %s", got, want)
		}
		if !slices.Contains(repo.links[activeShopID], int64(7)) {
			t.Errorf("links = %v, want the new burger linked to shop %d", repo.links[activeShopID], activeShopID)
		}
	})

	t.Run("正の burger_id は burger_name より優先される", func(t *testing.T) {
		repo := seedReviewWorld(1)
		router, aliceAuth, _, _ := newReviewsRouter(t, repo)
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":"Both","shop_id":%d,"burger_id":%d,"burger_name":"Veggie"}}`,
			activeShopID, cheeseBurgerID)
		rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
		}
		want := `{"id":1,"rating":4,"comment":"Both","created_at":"2024-06-01T12:01:00Z","photo_url":null,"user":{"id":1,"username":"alice"},` +
			`"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want the burger_id burger %s", got, want)
		}
		if len(repo.burgers) != 2 {
			t.Errorf("burgers = %d, want no burger created when burger_id wins", len(repo.burgers))
		}
	})

	t.Run("burger_id も使える burger_name も無い場合は 422 を返す", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
		bodies := map[string]string{
			"burger_id も burger_name も無い":    fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d}}`, activeShopID),
			"空白のみの burger_name":              fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_name":"  "}}`, activeShopID),
			"burger_id が 0 で burger_name が空": fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_id":0,"burger_name":""}}`, activeShopID),
		}
		for name, body := range bodies {
			t.Run(name, func(t *testing.T) {
				rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
				if rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
				}
				if got, want := rec.Body.String(), `{"errors":["Burger name can't be blank"]}`; got != want {
					t.Errorf("body = %s, want %s", got, want)
				}
			})
		}
	})

	t.Run("AC4 検証エラーは正確なメッセージ付きで 422 を返す", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
		post := func(rating int, comment string) *doResult {
			body := fmt.Sprintf(`{"review":{"rating":%d,"comment":%q,"shop_id":%d,"burger_id":%d}}`,
				rating, comment, activeShopID, cheeseBurgerID)
			rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
			return &doResult{code: rec.Code, body: rec.Body.String()}
		}
		tests := []struct {
			name     string
			rating   int
			comment  string
			wantBody string
		}{
			{name: "rating が 0 だと検証エラーになる", rating: 0, comment: "ok", wantBody: `{"errors":["Rating must be in 1..5"]}`},
			{name: "rating が 6 だと検証エラーになる", rating: 6, comment: "ok", wantBody: `{"errors":["Rating must be in 1..5"]}`},
			{name: "comment が空だと検証エラーになる", rating: 3, comment: "", wantBody: `{"errors":["Comment can't be blank"]}`},
			{name: "両方不正な場合は rating のメッセージが先に来る", rating: 0, comment: "",
				wantBody: `{"errors":["Rating must be in 1..5","Comment can't be blank"]}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := post(tt.rating, tt.comment)
				if got.code != http.StatusUnprocessableEntity {
					t.Fatalf("status = %d, want %d (body %s)", got.code, http.StatusUnprocessableEntity, got.body)
				}
				if got.body != tt.wantBody {
					t.Errorf("body = %s, want %s", got.body, tt.wantBody)
				}
			})
		}
	})
}

// doResult は、テーブル駆動テストの helper 用に、記録された status/body の
// 組を保持する。
type doResult struct {
	code int
	body string
}

// seedFeed は、標準の fixture の review を API 経由で投稿する：alice は Cheese
// （active な shop）と Plain（pending のみの shop）を review する。
func seedFeed(t *testing.T, router http.Handler, aliceAuth string) (cheeseReviewID, plainReviewID int64) {
	t.Helper()
	post := func(shopID, burgerID int64, comment string) {
		t.Helper()
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":%q,"shop_id":%d,"burger_id":%d}}`, comment, shopID, burgerID)
		if rec := do(router, http.MethodPost, "/reviews", body, aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("seed post: status = %d (body %s)", rec.Code, rec.Body)
		}
	}
	post(activeShopID, cheeseBurgerID, "On cheese")
	post(pendingShopID, plainBurgerID, "On plain")
	return 1, 2
}

// TestListReviews は AC5 と feed のセマンティクスを扱う：active な shop で
// 提供されている burger の review だけが現れ（active な link が重複していても
// それぞれちょうど 1 回だけ）、新しい順で、pagination される。
func TestListReviews(t *testing.T) {
	repo := seedReviewWorld(1)
	router, aliceAuth, _, _ := newReviewsRouter(t, repo)
	cheeseReviewID, plainReviewID := seedFeed(t, router, aliceAuth)

	t.Run("AC5 pending な shop にしか無い burger の review は匿名の feed に現れない", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/reviews", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := fmt.Sprintf(`[{"id":%d,"rating":4,"comment":"On cheese","created_at":"2024-06-01T12:01:00Z",`+
			`"photo_url":null,"user":{"id":1,"username":"alice"},"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}}]`,
			cheeseReviewID)
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("pending な shop にしか無い burger の review は投稿者本人にも隠れたままになる (viewer による絞り込みなし)", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/reviews", "", aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body %s)", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); got == "" || len(got) < 2 || got[0] != '[' {
			t.Fatalf("body = %s, want a JSON array", got)
		}
		if want := fmt.Sprintf(`"id":%d`, plainReviewID); containsJSONID(t, rec.Body.String(), plainReviewID) {
			t.Errorf("body %s unexpectedly contains %s", rec.Body.String(), want)
		}
	})

	t.Run("新しい順で pagination される", func(t *testing.T) {
		// 2 件目の cheese の review（id 3、より後の created_at）が
		// 先頭に来なければならない。
		body := fmt.Sprintf(`{"review":{"rating":5,"comment":"Again","shop_id":%d,"burger_id":%d}}`, active2ShopID, cheeseBurgerID)
		if rec := do(router, http.MethodPost, "/reviews", body, aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("post: status = %d (body %s)", rec.Code, rec.Body)
		}
		rec := do(router, http.MethodGet, "/reviews?per_page=1", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body %s)", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); !containsJSONID(t, got, 3) {
			t.Errorf("first page = %s, want the newest review (id 3)", got)
		}
		rec = do(router, http.MethodGet, "/reviews?per_page=1&page=2", "", "")
		if got := rec.Body.String(); !containsJSONID(t, got, cheeseReviewID) {
			t.Errorf("second page = %s, want review %d", got, cheeseReviewID)
		}
		rec = do(router, http.MethodGet, "/reviews?per_page=1&page=99", "", "")
		if got := rec.Body.String(); got != `[]` {
			t.Errorf("far page = %s, want []", got)
		}
		// 範囲外の整数（page=0）と大きすぎる値は、エラーにならず fallback / clamp
		// される（正確な clamp の値は usecase のテストで固定されている）。
		rec = do(router, http.MethodGet, "/reviews?page=0&per_page=9999", "", "")
		if rec.Code != http.StatusOK {
			t.Errorf("clamped request status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
	})

	t.Run("repository の失敗は 500 を返す", func(t *testing.T) {
		failRepo := newReviewRepoFake()
		failRepo.err = fmt.Errorf("db down")
		failRouter, _, _, _ := newReviewsRouter(t, failRepo)
		rec := do(failRouter, http.MethodGet, "/reviews", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}

// TestListReviewsFilters は GET /reviews の rating/keyword/shop_id/user_id の
// query filter（user_id 以外は Rails ReviewQuery parity）を扱う：各 filter 単独、
// それらの AND 結合、一致なしの場合の `[]`（決して null ではない）、空の値が
// 未指定として扱われること、そして整数でない rating/shop_id/user_id に対する
// fail-loud な 422（rating/shop_id については Rails の、黙って 0 に cast する
// 挙動からの意図的な乖離。user_id は Rails に対応物がなく同じ形に揃えている）。
func TestListReviewsFilters(t *testing.T) {
	repo := seedReviewWorld(1)
	router, aliceAuth, bobAuth, _ := newReviewsRouter(t, repo)
	// review 1：rating 4 の "On cheese"（Cheese、active な shop）。review 2：
	// rating 4 の "On plain"（Plain、pending のみ、feed からは隠される）。
	seedFeed(t, router, aliceAuth)
	// review 3：新しい Veggie burger（fake の id 7）に対する rating 5 の
	// "Smoky veggie dream"。その burger は 2 つ目の active な shop にのみ
	// 紐づく。
	body := fmt.Sprintf(`{"review":{"rating":5,"comment":"Smoky veggie dream","shop_id":%d,"burger_name":"Veggie"}}`, active2ShopID)
	if rec := do(router, http.MethodPost, "/reviews", body, aliceAuth); rec.Code != http.StatusCreated {
		t.Fatalf("seed veggie post: status = %d (body %s)", rec.Code, rec.Body)
	}
	// review 4：bob（id 2）が Cheese（active な shop）に投稿した rating 3 の
	// "Bob was here"。他の rating/keyword の検証には影響せず、user_id の絞り込みで
	// alice の review と区別される。
	body = fmt.Sprintf(`{"review":{"rating":3,"comment":"Bob was here","shop_id":%d,"burger_id":%d}}`, activeShopID, cheeseBurgerID)
	if rec := do(router, http.MethodPost, "/reviews", body, bobAuth); rec.Code != http.StatusCreated {
		t.Fatalf("seed bob post: status = %d (body %s)", rec.Code, rec.Body)
	}
	const (
		onCheese = `"On cheese"` // comment で review を識別する
		onPlain  = `"On plain"`  // （id は user/burger の id と衝突する）
		smoky    = `"Smoky veggie dream"`
		bobs     = `"Bob was here"`
	)

	get := func(t *testing.T, query string, want, absent []string) {
		t.Helper()
		rec := do(router, http.MethodGet, "/reviews"+query, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /reviews%s status = %d, want 200 (body %s)", query, rec.Code, rec.Body)
		}
		for _, comment := range want {
			if !strings.Contains(rec.Body.String(), comment) {
				t.Errorf("GET /reviews%s = %s, want the %s review", query, rec.Body, comment)
			}
		}
		for _, comment := range absent {
			if strings.Contains(rec.Body.String(), comment) {
				t.Errorf("GET /reviews%s = %s, the %s review must be filtered out", query, rec.Body, comment)
			}
		}
	}

	t.Run("rating は指定値と完全一致する review に絞り込まれる", func(t *testing.T) {
		get(t, "?rating=5", []string{smoky}, []string{onCheese, onPlain})
		get(t, "?rating=4", []string{onCheese}, []string{smoky, onPlain})
	})

	t.Run("keyword は comment に大文字小文字を区別せず一致する", func(t *testing.T) {
		get(t, "?keyword=SMOKY", []string{smoky}, []string{onCheese})
		get(t, "?keyword=On+cheese", []string{onCheese}, []string{smoky})
	})

	t.Run("shop_id はその shop の burger の review だけを残す", func(t *testing.T) {
		get(t, fmt.Sprintf("?shop_id=%d", activeShopID), []string{onCheese}, []string{smoky})
		get(t, fmt.Sprintf("?shop_id=%d", active2ShopID), []string{onCheese, smoky}, nil)
	})

	t.Run("user_id はその user の公開 review だけを残す", func(t *testing.T) {
		// alice（id 1）の pending な shop だけの review（onPlain）は、user_id を
		// 指定しても公開ルールにより現れない。
		get(t, "?user_id=1", []string{smoky, onCheese}, []string{bobs, onPlain})
		get(t, "?user_id=2", []string{bobs}, []string{smoky, onCheese, onPlain})
	})

	t.Run("user_id は整数として usecase に渡される", func(t *testing.T) {
		repo.listFilters = nil
		get(t, "?user_id=42", nil, nil)
		if len(repo.listFilters) != 1 || repo.listFilters[0].UserID == nil || *repo.listFilters[0].UserID != 42 {
			t.Errorf("filters = %+v, want exactly one with UserID 42", repo.listFilters)
		}
	})

	t.Run("空の user_id は未指定として usecase に渡される", func(t *testing.T) {
		repo.listFilters = nil
		get(t, "?user_id=", []string{smoky, onCheese, bobs}, []string{onPlain})
		if len(repo.listFilters) != 1 || repo.listFilters[0].UserID != nil {
			t.Errorf("filters = %+v, want exactly one with nil UserID", repo.listFilters)
		}
	})

	t.Run("filter は AND で結合される", func(t *testing.T) {
		get(t, fmt.Sprintf("?shop_id=%d&rating=5&keyword=veggie", active2ShopID), []string{smoky}, []string{onCheese})
		get(t, "?user_id=1&rating=4", []string{onCheese}, []string{smoky, bobs})
		get(t, "?user_id=2&rating=4", nil, []string{onCheese, smoky, bobs})
	})

	t.Run("一致なしは null ではなく空の JSON 配列を返す", func(t *testing.T) {
		for _, query := range []string{"?rating=2", "?keyword=zzz", "?shop_id=999", "?user_id=999", "?rating=5&keyword=cheese"} {
			rec := do(router, http.MethodGet, "/reviews"+query, "", "")
			if rec.Code != http.StatusOK || rec.Body.String() != `[]` {
				t.Errorf("GET /reviews%s = %d %s, want 200 []", query, rec.Code, rec.Body)
			}
		}
	})

	t.Run("空の filter 値は未指定として扱われる", func(t *testing.T) {
		get(t, "?rating=&keyword=&shop_id=&user_id=", []string{smoky, onCheese}, []string{onPlain})
	})

	t.Run("整数でない rating と shop_id と user_id は 422 で明示的に失敗し、usecase を呼ばない", func(t *testing.T) {
		tests := []struct {
			query    string
			wantBody string
		}{
			{query: "?rating=abc", wantBody: `{"errors":["Rating must be an integer"]}`},
			{query: "?rating=4.5", wantBody: `{"errors":["Rating must be an integer"]}`},
			{query: "?shop_id=abc", wantBody: `{"errors":["Shop id must be an integer"]}`},
			{query: "?user_id=abc", wantBody: `{"errors":["User id must be an integer"]}`},
			{query: "?user_id=1.5", wantBody: `{"errors":["User id must be an integer"]}`},
			// int64 を超える値も整数として受け付けない。
			{query: "?user_id=9223372036854775808", wantBody: `{"errors":["User id must be an integer"]}`},
			// パースの順序は rating、shop_id、user_id である。
			{query: "?rating=abc&shop_id=abc&user_id=abc", wantBody: `{"errors":["Rating must be an integer"]}`},
			{query: "?shop_id=abc&user_id=abc", wantBody: `{"errors":["Shop id must be an integer"]}`},
			{query: "?rating=4&shop_id=1&user_id=abc", wantBody: `{"errors":["User id must be an integer"]}`},
		}
		repo.listFilters = nil
		for _, tt := range tests {
			rec := do(router, http.MethodGet, "/reviews"+tt.query, "", "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("GET /reviews%s status = %d, want 422 (body %s)", tt.query, rec.Code, rec.Body)
				continue
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("GET /reviews%s body = %s, want %s", tt.query, got, tt.wantBody)
			}
		}
		if len(repo.listFilters) != 0 {
			t.Errorf("ListReviews was called %d times, want 0 for invalid filters", len(repo.listFilters))
		}
	})
}

// containsJSONID は、body（review の JSON 配列）の要素のうち、最上位の id が
// 引数の id と一致するものがあるかどうかを返す。埋め込まれた user や burger の
// id には一致しない。body が JSON 配列として解釈できない場合は、「含まれない」
// 系の検査が空振りで通ってしまわないよう、テストを失敗させる（null は空配列として
// 扱われるが、handler は null を返さない）。
func containsJSONID(t *testing.T, body string, id int64) bool {
	t.Helper()
	var reviews []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &reviews); err != nil {
		t.Fatalf("body is not a JSON array of reviews: %v (body %s)", err, body)
	}
	for _, r := range reviews {
		if r.ID == id {
			return true
		}
	}
	return false
}

// TestGetReviewNotFound は一様な review の 404 を扱う：未知の id、数値でない
// id、discard 済みの review は、まったく同じ body を共有する。
func TestGetReviewNotFound(t *testing.T) {
	router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	if rec := do(router, http.MethodDelete, fmt.Sprintf("/reviews/%d", cheeseReviewID), "", aliceAuth); rec.Code != http.StatusNoContent {
		t.Fatalf("seed delete: status = %d (body %s)", rec.Code, rec.Body)
	}

	const notFoundBody = `{"error":"Review not found"}`
	for _, path := range []string{"/reviews/999", "/reviews/abc", fmt.Sprintf("/reviews/%d", cheeseReviewID)} {
		rec := do(router, http.MethodGet, path, "", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404 (body %s)", path, rec.Code, rec.Body)
		}
		if got := rec.Body.String(); got != notFoundBody {
			t.Errorf("GET %s body = %q, want %q", path, got, notFoundBody)
		}
	}
}

// TestUpdateReview は HTTP レベルで AC3 を扱う：author は 200 で編集でき、その
// 変更が反映される。それ以外の全員（admin を含む）は 403 になり、validation の
// 失敗は 422、未知の review は 404 になる。
func TestUpdateReview(t *testing.T) {
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	path := fmt.Sprintf("/reviews/%d", cheeseReviewID)
	editBody := `{"review":{"rating":5,"comment":"Even better"}}`

	t.Run("AC3 他人の review は 403 を返す", func(t *testing.T) {
		for name, auth := range map[string]string{"other user": bobAuth, "admin": adminAuth} {
			rec := do(router, http.MethodPut, path, editBody, auth)
			if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
				t.Errorf("%s: status/body = %d %s, want 403 Forbidden", name, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC3 投稿者が編集すると 200 を返し、変更が反映される", func(t *testing.T) {
		rec := do(router, http.MethodPut, path, editBody, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := fmt.Sprintf(`{"id":%d,"rating":5,"comment":"Even better","created_at":"2024-06-01T12:01:00Z",`+
			`"photo_url":null,"user":{"id":1,"username":"alice"},"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}}`,
			cheeseReviewID)
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
		if detail := do(router, http.MethodGet, path, "", ""); detail.Body.String() != want {
			t.Errorf("detail after edit = %s, want %s", detail.Body, want)
		}
	})

	t.Run("AC4 不正な内容は 422 を返す", func(t *testing.T) {
		rec := do(router, http.MethodPut, path, `{"review":{"rating":6,"comment":""}}`, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Rating must be in 1..5","Comment can't be blank"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("未知の id と数値でない id は review の 404 を返す", func(t *testing.T) {
		for _, p := range []string{"/reviews/999", "/reviews/abc"} {
			rec := do(router, http.MethodPut, p, editBody, aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Review not found"}` {
				t.Errorf("PUT %s = %d %s, want 404 Review not found", p, rec.Code, rec.Body)
			}
		}
	})
}

// TestUpdateReviewIgnoresShopAndBurgerID は PUT の改ざんに関するルールを
// 固定する：review は別の shop や burger に移ることが決してないので、edit の
// body に含まれる shop_id/burger_id は黙って無視される。response（とその後の
// GET）は、rating/comment だけが更新された元の burger を示し続ける。
func TestUpdateReviewIgnoresShopAndBurgerID(t *testing.T) {
	router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	path := fmt.Sprintf("/reviews/%d", cheeseReviewID)

	// pendingShopID/plainBurgerID は存在するが、その review の元の active な
	// shop と Cheese burger とは異なる。
	body := fmt.Sprintf(`{"review":{"rating":2,"comment":"Tampered","shop_id":%d,"burger_id":%d}}`,
		pendingShopID, plainBurgerID)
	rec := do(router, http.MethodPut, path, body, aliceAuth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := fmt.Sprintf(`{"id":%d,"rating":2,"comment":"Tampered","created_at":"2024-06-01T12:01:00Z",`+
		`"photo_url":null,"user":{"id":1,"username":"alice"},"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}}`,
		cheeseReviewID)
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s, want the original burger with updated content %s", got, want)
	}
	if detail := do(router, http.MethodGet, path, "", ""); detail.Body.String() != want {
		t.Errorf("detail after tampered edit = %s, want %s", detail.Body, want)
	}

	// でたらめな id も同様に効果を持たない。
	rec = do(router, http.MethodPut, path, `{"review":{"rating":2,"comment":"Tampered","shop_id":999,"burger_id":888}}`, aliceAuth)
	if rec.Code != http.StatusOK {
		t.Fatalf("bogus ids: status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	if got := rec.Body.String(); got != want {
		t.Errorf("bogus ids: body = %s, want %s", got, want)
	}
}

// TestDeleteReview は HTTP レベルで AC3/AC6 を扱う：soft delete できるのは
// author だけであり（204、body なし）、その後、その review は detail と
// feed から消え、2 回目の delete は 404 になる。
func TestDeleteReview(t *testing.T) {
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	path := fmt.Sprintf("/reviews/%d", cheeseReviewID)

	t.Run("AC3 他人の review は 403 を返す", func(t *testing.T) {
		for name, auth := range map[string]string{"other user": bobAuth, "admin": adminAuth} {
			rec := do(router, http.MethodDelete, path, "", auth)
			if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
				t.Errorf("%s: status/body = %d %s, want 403 Forbidden", name, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC6 投稿者が削除すると body なしの 204 を返し、その後 review は消える", func(t *testing.T) {
		rec := do(router, http.MethodDelete, path, "", aliceAuth)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNoContent, rec.Body)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty", rec.Body.String())
		}
		if detail := do(router, http.MethodGet, path, "", ""); detail.Code != http.StatusNotFound {
			t.Errorf("detail after delete = %d, want 404", detail.Code)
		}
		if list := do(router, http.MethodGet, "/reviews", "", ""); containsJSONID(t, list.Body.String(), cheeseReviewID) {
			t.Errorf("feed after delete still contains review %d: %s", cheeseReviewID, list.Body)
		}
		if again := do(router, http.MethodDelete, path, "", aliceAuth); again.Code != http.StatusNotFound {
			t.Errorf("second delete = %d, want 404", again.Code)
		}
	})
}

// TestReviewsRequireAuth は 401 の境界を固定する：トークンなしのすべての
// 書き込みは repository へのアクセスより前に拒否され、一方で読み取りは
// 開かれたままである。
func TestReviewsRequireAuth(t *testing.T) {
	router, _, _, _ := newReviewsRouter(t, seedReviewWorld(1))
	const unauthorized = `{"error":"Unauthorized"}`

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/reviews", body: `{"review":{"rating":4,"comment":"ok","shop_id":1,"burger_id":5}}`},
		{method: http.MethodPut, path: "/reviews/1", body: `{"review":{"rating":4,"comment":"ok"}}`},
		{method: http.MethodDelete, path: "/reviews/1"},
	}
	for _, tt := range tests {
		rec := do(router, tt.method, tt.path, tt.body, "")
		if rec.Code != http.StatusUnauthorized || rec.Body.String() != unauthorized {
			t.Errorf("%s %s = %d %s, want 401 %s", tt.method, tt.path, rec.Code, rec.Body, unauthorized)
		}
	}

	for _, path := range []string{"/reviews", "/reviews/999"} {
		rec := do(router, http.MethodGet, path, "", "")
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("GET %s unexpectedly requires auth", path)
		}
	}
}
