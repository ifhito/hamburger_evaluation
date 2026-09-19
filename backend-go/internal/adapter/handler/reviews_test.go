package handler_test

import (
	"context"
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

// fakeStoredReview is one review row inside reviewRepoFake.
type fakeStoredReview struct {
	review    domain.Review
	discarded bool
}

// reviewRepoFake is an in-memory usecase.ReviewRepository. The active-shop
// feed filter is derived from the seeded shops and links (mirroring the
// SQL EXISTS; the SQL itself is covered by the repository integration
// tests). Setting err fails every operation (500 paths).
type reviewRepoFake struct {
	shops     map[int64]domain.Shop
	links     map[int64][]int64 // shopID -> linked burger ids
	burgers   map[int64]domain.ShopReviewBurger
	usernames map[int64]string
	seq       int64
	reviews   map[int64]*fakeStoredReview
	err       error
}

func newReviewRepoFake() *reviewRepoFake {
	return &reviewRepoFake{
		shops:     map[int64]domain.Shop{},
		links:     map[int64][]int64{},
		burgers:   map[int64]domain.ShopReviewBurger{},
		usernames: map[int64]string{},
		reviews:   map[int64]*fakeStoredReview{},
	}
}

// reviewBaseTime anchors the deterministic created_at values (creation n
// gets reviewBaseTime + n minutes).
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
	// The filter mirrors the SQL narg predicates: exact rating, literal
	// case-insensitive comment substring, shops_burgers link (the exact SQL
	// is covered by the repository integration tests).
	matches := func(review domain.Review) bool {
		if filter.Rating != nil && review.Rating != *filter.Rating {
			return false
		}
		if filter.Keyword != "" && (review.Comment == nil ||
			!strings.Contains(strings.ToLower(*review.Comment), strings.ToLower(filter.Keyword))) {
			return false
		}
		// Like the SQL, the filter shop must itself be active.
		if filter.ShopID != nil && (f.shops[*filter.ShopID].Status != domain.ShopStatusActive ||
			!slices.Contains(f.links[*filter.ShopID], review.BurgerID)) {
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
		// Lowest id wins, mirroring the SQL's ORDER BY b.id LIMIT 1.
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

// seedReviewWorld fills the fake with the S6 fixture: an active, a
// pending (created by creatorID), and a rejected shop; a Cheese burger
// (with stats) linked to both active shops and a Plain burger (no stats)
// linked only to the pending shop.
const (
	activeShopID   = int64(1)
	pendingShopID  = int64(2)
	rejectedShopID = int64(3)
	active2ShopID  = int64(4)
	cheeseBurgerID = int64(5)
	plainBurgerID  = int64(6)
)

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
	// Cheese is served by BOTH active shops (the duplicate-link case) and
	// by the pending shop; Plain only by the pending shop.
	repo.links[activeShopID] = []int64{cheeseBurgerID}
	repo.links[active2ShopID] = []int64{cheeseBurgerID}
	repo.links[pendingShopID] = []int64{cheeseBurgerID, plainBurgerID}
	repo.links[rejectedShopID] = []int64{cheeseBurgerID}
	return repo
}

// newReviewsRouter wires the router with the auth kit and the given
// review fake, returning Bearer headers for alice (id 1), bob (id 2), and
// an admin (id 3). The fake's usernames map is aligned with those ids.
func newReviewsRouter(t *testing.T, repo *reviewRepoFake) (router http.Handler, aliceAuth, bobAuth, adminAuth string) {
	t.Helper()
	router, _, aliceAuth, bobAuth, adminAuth = newPhotoReviewsRouter(t, repo)
	return router, aliceAuth, bobAuth, adminAuth
}

// newPhotoReviewsRouter is newReviewsRouter plus the S10 photo wiring: a
// real disk store rooted in a fresh temp dir (returned for file
// assertions), served under GET /photos/ through the same
// handler.PhotoFileServer wrapper cmd/api wires in disk mode.
func newPhotoReviewsRouter(t *testing.T, repo *reviewRepoFake) (router http.Handler, photoDir, aliceAuth, bobAuth, adminAuth string) {
	t.Helper()
	users, auth, codec := newAuthKit()
	alice := users.seed("alice", "alice@example.com", "password123")
	bob := users.seed("bob", "bob@example.com", "password123")
	admin := users.seed("root", "root@example.com", "password123")
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
	router = handler.NewRouter(okPinger, auth, usecase.NewShops(&shopRepoFake{}),
		usecase.NewReviews(repo, storage.NewDisk(photoDir, "/photos")),
		usecase.NewUsers(users, hasherFake{}), handler.PhotoFileServer(photoDir))
	return router, photoDir, token(alice.ID), token(bob.ID), token(admin.ID)
}

// TestCreateReview covers AC1 and AC2 at the HTTP level: posting to an
// active shop yields 201 with the exact payload and the review appears in
// the feed and detail; rejected and foreign-pending shops yield 403 while
// the creator and an admin may post to a pending shop.
func TestCreateReview(t *testing.T) {
	t.Run("AC1 authenticated post to active shop returns 201 and appears in list and detail", func(t *testing.T) {
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

	t.Run("AC2 reviewable rule: rejected 403 for all, pending only for creator and admin", func(t *testing.T) {
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
			{name: "creator posting to rejected shop gets 403", auth: aliceAuth, shopID: rejectedShopID, wantCode: http.StatusForbidden},
			{name: "admin posting to rejected shop gets 403", auth: adminAuth, shopID: rejectedShopID, wantCode: http.StatusForbidden},
			{name: "creator posting to own pending shop gets 201", auth: aliceAuth, shopID: pendingShopID, wantCode: http.StatusCreated},
			{name: "other user posting to pending shop gets 403", auth: bobAuth, shopID: pendingShopID, wantCode: http.StatusForbidden},
			{name: "admin posting to pending shop gets 201", auth: adminAuth, shopID: pendingShopID, wantCode: http.StatusCreated},
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

	t.Run("unknown shop and unlinked burger yield their 404 bodies", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
		rec := do(router, http.MethodPost, "/reviews",
			fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":999,"burger_id":%d}}`, cheeseBurgerID), aliceAuth)
		if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Shop not found"}` {
			t.Errorf("unknown shop = %d %s, want 404 Shop not found", rec.Code, rec.Body)
		}
		// Plain exists but is not served by the active shop.
		rec = do(router, http.MethodPost, "/reviews",
			fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_id":%d}}`, activeShopID, plainBurgerID), aliceAuth)
		if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Burger not found"}` {
			t.Errorf("unlinked burger = %d %s, want 404 Burger not found", rec.Code, rec.Body)
		}
	})

	t.Run("burger_name reuses the shop's burger of that name (S6 P3-1)", func(t *testing.T) {
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

	t.Run("unknown burger_name creates the burger and its link (S6 P3-1)", func(t *testing.T) {
		repo := seedReviewWorld(1)
		router, aliceAuth, _, _ := newReviewsRouter(t, repo)
		body := fmt.Sprintf(`{"review":{"rating":5,"comment":"New","shop_id":%d,"burger_name":"Veggie"}}`, activeShopID)
		rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
		}
		// The created burger (fake id 7) carries zero stats.
		want := `{"id":1,"rating":5,"comment":"New","created_at":"2024-06-01T12:01:00Z","photo_url":null,"user":{"id":1,"username":"alice"},` +
			`"burger":{"id":7,"name":"Veggie","average_rating":0,"review_count":0,"weighted_score":0,"confidence":0}}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want the created burger %s", got, want)
		}
		if !slices.Contains(repo.links[activeShopID], int64(7)) {
			t.Errorf("links = %v, want the new burger linked to shop %d", repo.links[activeShopID], activeShopID)
		}
	})

	t.Run("a positive burger_id wins over burger_name", func(t *testing.T) {
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

	t.Run("neither burger_id nor a usable burger_name returns 422", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
		bodies := map[string]string{
			"neither field":         fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d}}`, activeShopID),
			"whitespace-only name":  fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_name":"  "}}`, activeShopID),
			"burger_id 0 and blank": fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%d,"burger_id":0,"burger_name":""}}`, activeShopID),
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

	t.Run("AC4 validation failures return 422 with the exact messages", func(t *testing.T) {
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
			{name: "rating 0", rating: 0, comment: "ok", wantBody: `{"errors":["Rating must be in 1..5"]}`},
			{name: "rating 6", rating: 6, comment: "ok", wantBody: `{"errors":["Rating must be in 1..5"]}`},
			{name: "blank comment", rating: 3, comment: "", wantBody: `{"errors":["Comment can't be blank"]}`},
			{name: "both invalid, rating message first", rating: 0, comment: "",
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

// doResult carries a recorded status/body pair for table helpers.
type doResult struct {
	code int
	body string
}

// seedFeed posts the standard fixture reviews through the API: alice
// reviews Cheese (active shops) and Plain (pending-only shop).
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

// TestListReviews covers AC5 and the feed semantics: only reviews of
// burgers served by an active shop appear (each exactly once despite the
// duplicate active link), newest first, paginated.
func TestListReviews(t *testing.T) {
	repo := seedReviewWorld(1)
	router, aliceAuth, _, _ := newReviewsRouter(t, repo)
	cheeseReviewID, plainReviewID := seedFeed(t, router, aliceAuth)

	t.Run("AC5 pending-only burger review is absent from the anonymous feed", func(t *testing.T) {
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

	t.Run("pending-only review stays hidden even for its author (no viewer filtering)", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/reviews", "", aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body %s)", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); got == "" || len(got) < 2 || got[0] != '[' {
			t.Fatalf("body = %s, want a JSON array", got)
		}
		if want := fmt.Sprintf(`"id":%d`, plainReviewID); containsJSONID(rec.Body.String(), plainReviewID) {
			t.Errorf("body %s unexpectedly contains %s", rec.Body.String(), want)
		}
	})

	t.Run("newest first and paginated", func(t *testing.T) {
		// A second cheese review (id 3, later created_at) must come first.
		body := fmt.Sprintf(`{"review":{"rating":5,"comment":"Again","shop_id":%d,"burger_id":%d}}`, active2ShopID, cheeseBurgerID)
		if rec := do(router, http.MethodPost, "/reviews", body, aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("post: status = %d (body %s)", rec.Code, rec.Body)
		}
		rec := do(router, http.MethodGet, "/reviews?per_page=1", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body %s)", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); !containsJSONID(got, 3) {
			t.Errorf("first page = %s, want the newest review (id 3)", got)
		}
		rec = do(router, http.MethodGet, "/reviews?per_page=1&page=2", "", "")
		if got := rec.Body.String(); !containsJSONID(got, cheeseReviewID) {
			t.Errorf("second page = %s, want review %d", got, cheeseReviewID)
		}
		rec = do(router, http.MethodGet, "/reviews?per_page=1&page=99", "", "")
		if got := rec.Body.String(); got != `[]` {
			t.Errorf("far page = %s, want []", got)
		}
		// Non-numeric and oversized values fall back / clamp instead of
		// erroring (exact clamp values are pinned in the usecase tests).
		rec = do(router, http.MethodGet, "/reviews?page=abc&per_page=9999", "", "")
		if rec.Code != http.StatusOK {
			t.Errorf("clamped request status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
	})

	t.Run("repository failure returns 500", func(t *testing.T) {
		failRepo := newReviewRepoFake()
		failRepo.err = fmt.Errorf("db down")
		failRouter, _, _, _ := newReviewsRouter(t, failRepo)
		rec := do(failRouter, http.MethodGet, "/reviews", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}

// TestListReviewsFilters covers the GET /reviews rating/keyword/shop_id
// query filters (Rails ReviewQuery parity): each filter alone, their AND
// combination, a no-match `[]` (never null), empty values counting as
// absent, and the fail-loud 422 for non-integer rating/shop_id (a
// deliberate divergence from Rails' silent cast-to-0).
func TestListReviewsFilters(t *testing.T) {
	repo := seedReviewWorld(1)
	router, aliceAuth, _, _ := newReviewsRouter(t, repo)
	// Review 1: rating 4 "On cheese" (Cheese, active shops); review 2:
	// rating 4 "On plain" (Plain, pending-only, hidden from the feed).
	seedFeed(t, router, aliceAuth)
	// Review 3: rating 5 "Smoky veggie dream" on a new Veggie burger (fake
	// id 7) linked only to the second active shop.
	body := fmt.Sprintf(`{"review":{"rating":5,"comment":"Smoky veggie dream","shop_id":%d,"burger_name":"Veggie"}}`, active2ShopID)
	if rec := do(router, http.MethodPost, "/reviews", body, aliceAuth); rec.Code != http.StatusCreated {
		t.Fatalf("seed veggie post: status = %d (body %s)", rec.Code, rec.Body)
	}
	const (
		onCheese = `"On cheese"` // the comments identify the reviews
		onPlain  = `"On plain"`  // (ids collide with user/burger ids)
		smoky    = `"Smoky veggie dream"`
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

	t.Run("rating filters to the exact rating", func(t *testing.T) {
		get(t, "?rating=5", []string{smoky}, []string{onCheese, onPlain})
		get(t, "?rating=4", []string{onCheese}, []string{smoky, onPlain})
	})

	t.Run("keyword matches the comment case-insensitively", func(t *testing.T) {
		get(t, "?keyword=SMOKY", []string{smoky}, []string{onCheese})
		get(t, "?keyword=On+cheese", []string{onCheese}, []string{smoky})
	})

	t.Run("shop_id keeps only that shop's burgers' reviews", func(t *testing.T) {
		get(t, fmt.Sprintf("?shop_id=%d", activeShopID), []string{onCheese}, []string{smoky})
		get(t, fmt.Sprintf("?shop_id=%d", active2ShopID), []string{onCheese, smoky}, nil)
	})

	t.Run("filters combine with AND", func(t *testing.T) {
		get(t, fmt.Sprintf("?shop_id=%d&rating=5&keyword=veggie", active2ShopID), []string{smoky}, []string{onCheese})
	})

	t.Run("no match returns the empty JSON array, never null", func(t *testing.T) {
		for _, query := range []string{"?rating=2", "?keyword=zzz", "?shop_id=999", "?rating=5&keyword=cheese"} {
			rec := do(router, http.MethodGet, "/reviews"+query, "", "")
			if rec.Code != http.StatusOK || rec.Body.String() != `[]` {
				t.Errorf("GET /reviews%s = %d %s, want 200 []", query, rec.Code, rec.Body)
			}
		}
	})

	t.Run("empty filter values count as absent", func(t *testing.T) {
		get(t, "?rating=&keyword=&shop_id=", []string{smoky, onCheese}, []string{onPlain})
	})

	t.Run("non-integer rating and shop_id fail loudly with 422", func(t *testing.T) {
		tests := []struct {
			query    string
			wantBody string
		}{
			{query: "?rating=abc", wantBody: `{"errors":["Rating must be an integer"]}`},
			{query: "?rating=4.5", wantBody: `{"errors":["Rating must be an integer"]}`},
			{query: "?shop_id=abc", wantBody: `{"errors":["Shop id must be an integer"]}`},
		}
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
	})
}

// containsJSONID reports whether body contains the "id":<id> pair.
func containsJSONID(body string, id int64) bool {
	needle := fmt.Sprintf(`"id":%d,`, id)
	for i := 0; i+len(needle) <= len(body); i++ {
		if body[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestGetReviewNotFound covers the uniform review 404: unknown ids,
// non-numeric ids, and discarded reviews share the exact body.
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

// TestUpdateReview covers AC3 at the HTTP level: the author edits with
// 200 and the change is reflected; everyone else (admin included) gets
// 403; validation failures 422; unknown reviews 404.
func TestUpdateReview(t *testing.T) {
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	path := fmt.Sprintf("/reviews/%d", cheeseReviewID)
	editBody := `{"review":{"rating":5,"comment":"Even better"}}`

	t.Run("AC3 someone else's review returns 403", func(t *testing.T) {
		for name, auth := range map[string]string{"other user": bobAuth, "admin": adminAuth} {
			rec := do(router, http.MethodPut, path, editBody, auth)
			if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
				t.Errorf("%s: status/body = %d %s, want 403 Forbidden", name, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC3 author edit returns 200 and is reflected", func(t *testing.T) {
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

	t.Run("AC4 invalid content returns 422", func(t *testing.T) {
		rec := do(router, http.MethodPut, path, `{"review":{"rating":6,"comment":""}}`, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Rating must be in 1..5","Comment can't be blank"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("unknown and non-numeric ids return the review 404", func(t *testing.T) {
		for _, p := range []string{"/reviews/999", "/reviews/abc"} {
			rec := do(router, http.MethodPut, p, editBody, aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Review not found"}` {
				t.Errorf("PUT %s = %d %s, want 404 Review not found", p, rec.Code, rec.Body)
			}
		}
	})
}

// TestUpdateReviewIgnoresShopAndBurgerID pins the PUT tampering rule: a
// review never moves to another shop or burger, so shop_id/burger_id in
// an edit body are silently ignored — the response (and a subsequent GET)
// still shows the original burger with only rating/comment updated.
func TestUpdateReviewIgnoresShopAndBurgerID(t *testing.T) {
	router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	path := fmt.Sprintf("/reviews/%d", cheeseReviewID)

	// pendingShopID/plainBurgerID exist but differ from the review's
	// original active shop and Cheese burger.
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

	// Bogus ids are just as inert.
	rec = do(router, http.MethodPut, path, `{"review":{"rating":2,"comment":"Tampered","shop_id":999,"burger_id":888}}`, aliceAuth)
	if rec.Code != http.StatusOK {
		t.Fatalf("bogus ids: status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	if got := rec.Body.String(); got != want {
		t.Errorf("bogus ids: body = %s, want %s", got, want)
	}
}

// TestDeleteReview covers AC3/AC6 at the HTTP level: only the author may
// soft-delete (204, no body); afterwards the review is gone from detail
// and feed, and a second delete 404s.
func TestDeleteReview(t *testing.T) {
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, seedReviewWorld(1))
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth)
	path := fmt.Sprintf("/reviews/%d", cheeseReviewID)

	t.Run("AC3 someone else's review returns 403", func(t *testing.T) {
		for name, auth := range map[string]string{"other user": bobAuth, "admin": adminAuth} {
			rec := do(router, http.MethodDelete, path, "", auth)
			if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
				t.Errorf("%s: status/body = %d %s, want 403 Forbidden", name, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC6 author delete returns 204 without a body, then the review is gone", func(t *testing.T) {
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
		if list := do(router, http.MethodGet, "/reviews", "", ""); containsJSONID(list.Body.String(), cheeseReviewID) {
			t.Errorf("feed after delete still contains review %d: %s", cheeseReviewID, list.Body)
		}
		if again := do(router, http.MethodDelete, path, "", aliceAuth); again.Code != http.StatusNotFound {
			t.Errorf("second delete = %d, want 404", again.Code)
		}
	})
}

// TestReviewsRequireAuth pins the 401 boundary: every write without a
// token is rejected before any repository access, while the reads stay
// open.
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
