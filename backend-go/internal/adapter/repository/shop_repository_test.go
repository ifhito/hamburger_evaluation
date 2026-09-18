package repository_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// insertRow inserts via sql (which must RETURN id) and returns the new id.
func insertRow(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := conn.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

// shopIDs extracts the ids of shops for order-insensitive comparisons.
func shopIDs(shops []domain.Shop) []int64 {
	ids := make([]int64, 0, len(shops))
	for _, s := range shops {
		ids = append(ids, s.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedIDs(ids ...int64) []int64 {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// shopNames extracts names preserving order, for keyword/order assertions.
func shopNames(shops []domain.Shop) []string {
	names := make([]string, 0, len(shops))
	for _, s := range shops {
		names = append(names, s.Name)
	}
	return names
}

// TestShopRepository exercises the shop read repository against a real
// PostgreSQL via the shared dbtest scaffold (skips without
// TEST_DATABASE_URL). It covers issue #12 AC1–AC3 and AC5 at the SQL
// level plus pagination, ordering, and both detail queries.
func TestShopRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewShopRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := insertRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status codes: 0=pending, 1=active, 2=rejected.
	deltaDiner := insertRow(ctx, t, conn, insertShop, "Delta Diner", 1, nil, carol)
	alicePending := insertRow(ctx, t, conn, insertShop, "Alpha Pending", 0, nil, alice)
	golfRejected := insertRow(ctx, t, conn, insertShop, "Golf Grill", 2, "needs fixes", nil)
	pctBeef := insertRow(ctx, t, conn, insertShop, "100% Beef", 1, nil, nil)
	xBeef := insertRow(ctx, t, conn, insertShop, "100x Beef", 1, nil, nil)
	underScore := insertRow(ctx, t, conn, insertShop, "Under_score", 1, nil, nil)
	underX := insertRow(ctx, t, conn, insertShop, "UnderXscore", 1, nil, nil)
	backslash := insertRow(ctx, t, conn, insertShop, `Back\slash Cafe`, 1, nil, nil)
	orderA1 := insertRow(ctx, t, conn, insertShop, "Order Cafe A", 1, nil, nil)
	orderA2 := insertRow(ctx, t, conn, insertShop, "Order Cafe A", 1, nil, nil)
	orderB := insertRow(ctx, t, conn, insertShop, "Order Cafe B", 1, nil, nil)

	activeIDs := sortedIDs(deltaDiner, pctBeef, xBeef, underScore, underX, backslash, orderA1, orderA2, orderB)

	anon := domain.ShopVisibilityFor(nil)
	aliceVis := domain.ShopVisibilityFor(&domain.User{ID: alice})
	adminVis := domain.ShopVisibilityFor(&domain.User{ID: 999, Admin: true})

	list := func(t *testing.T, vis domain.ShopVisibility, keyword string, limit, offset int32) []domain.Shop {
		t.Helper()
		shops, err := repo.ListShops(ctx, vis, keyword, limit, offset)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		return shops
	}

	t.Run("AC1 anonymous viewer lists active shops only", func(t *testing.T) {
		if got := shopIDs(list(t, anon, "", 100, 0)); !reflect.DeepEqual(got, activeIDs) {
			t.Errorf("ids = %v, want %v", got, activeIDs)
		}
	})

	t.Run("AC2 creator additionally sees own pending shop with its status", func(t *testing.T) {
		shops := list(t, aliceVis, "", 100, 0)
		want := sortedIDs(append([]int64{alicePending}, activeIDs...)...)
		if got := shopIDs(shops); !reflect.DeepEqual(got, want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
		for _, s := range shops {
			if s.ID == alicePending && s.Status != domain.ShopStatusPending {
				t.Errorf("pending shop status = %q, want %q", s.Status, domain.ShopStatusPending)
			}
		}
	})

	t.Run("AC3 admin sees all statuses", func(t *testing.T) {
		want := sortedIDs(append([]int64{alicePending, golfRejected}, activeIDs...)...)
		if got := shopIDs(list(t, adminVis, "", 100, 0)); !reflect.DeepEqual(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})

	t.Run("ordering is name asc then id asc", func(t *testing.T) {
		shops := list(t, anon, "Order Cafe", 100, 0)
		wantNames := []string{"Order Cafe A", "Order Cafe A", "Order Cafe B"}
		if got := shopNames(shops); !reflect.DeepEqual(got, wantNames) {
			t.Fatalf("names = %v, want %v", got, wantNames)
		}
		if shops[0].ID != orderA1 || shops[1].ID != orderA2 {
			t.Errorf("same-name ids = %d,%d, want %d,%d (id asc)", shops[0].ID, shops[1].ID, orderA1, orderA2)
		}
	})

	t.Run("pagination slices the ordered list and runs out to empty", func(t *testing.T) {
		if got := shopNames(list(t, anon, "Order Cafe", 2, 0)); !reflect.DeepEqual(got, []string{"Order Cafe A", "Order Cafe A"}) {
			t.Errorf("page 1 = %v", got)
		}
		if got := shopNames(list(t, anon, "Order Cafe", 2, 2)); !reflect.DeepEqual(got, []string{"Order Cafe B"}) {
			t.Errorf("page 2 = %v", got)
		}
		if got := list(t, anon, "Order Cafe", 2, 100); len(got) != 0 {
			t.Errorf("far page = %v, want empty", got)
		}
	})

	t.Run("AC5 keyword is case-insensitive substring", func(t *testing.T) {
		want := sortedIDs(pctBeef, xBeef)
		if got := shopIDs(list(t, anon, "bEEf", 100, 0)); !reflect.DeepEqual(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})

	t.Run("AC5 percent in keyword matches literally", func(t *testing.T) {
		if got := shopNames(list(t, anon, "100%", 100, 0)); !reflect.DeepEqual(got, []string{"100% Beef"}) {
			t.Errorf("names = %v, want [100%% Beef]", got)
		}
	})

	t.Run("AC5 underscore in keyword matches literally", func(t *testing.T) {
		if got := shopNames(list(t, anon, "Under_", 100, 0)); !reflect.DeepEqual(got, []string{"Under_score"}) {
			t.Errorf("names = %v, want [Under_score]", got)
		}
	})

	t.Run("AC5 backslash in keyword matches literally", func(t *testing.T) {
		if got := shopNames(list(t, anon, `\`, 100, 0)); !reflect.DeepEqual(got, []string{`Back\slash Cafe`}) {
			t.Errorf(`names = %v, want [Back\slash Cafe]`, got)
		}
	})

	t.Run("AC5 SQL injection keyword neither errors nor leaks", func(t *testing.T) {
		if got := list(t, anon, `'; DROP TABLE shops;--`, 100, 0); len(got) != 0 {
			t.Errorf("shops = %v, want empty", got)
		}
		// The table must still be intact.
		if got := shopIDs(list(t, anon, "", 100, 0)); !reflect.DeepEqual(got, activeIDs) {
			t.Errorf("ids after injection attempt = %v, want %v", got, activeIDs)
		}
	})

	t.Run("GetShopWithCreator returns creator and status", func(t *testing.T) {
		detail, err := repo.GetShopWithCreator(ctx, deltaDiner)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if detail.Name != "Delta Diner" || detail.Status != domain.ShopStatusActive {
			t.Errorf("shop = %+v, want Delta Diner/active", detail.Shop)
		}
		if detail.ModerationNote != nil {
			t.Errorf("ModerationNote = %v, want nil", *detail.ModerationNote)
		}
		want := &domain.UserRef{ID: carol, Username: "carol"}
		if !reflect.DeepEqual(detail.Creator, want) {
			t.Errorf("creator = %+v, want %+v", detail.Creator, want)
		}
	})

	t.Run("GetShopWithCreator maps null creator and moderation note", func(t *testing.T) {
		detail, err := repo.GetShopWithCreator(ctx, golfRejected)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if detail.Creator != nil {
			t.Errorf("creator = %+v, want nil", detail.Creator)
		}
		if detail.Status != domain.ShopStatusRejected {
			t.Errorf("status = %q, want rejected", detail.Status)
		}
		if detail.ModerationNote == nil || *detail.ModerationNote != "needs fixes" {
			t.Errorf("ModerationNote = %v, want needs fixes", detail.ModerationNote)
		}
	})

	t.Run("AC6 unknown shop id yields ErrShopNotFound", func(t *testing.T) {
		if _, err := repo.GetShopWithCreator(ctx, 99999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("ListShopReviews joins user, burger, and stats in order", func(t *testing.T) {
		insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
		cheese := insertRow(ctx, t, conn, insertBurger, "Cheese")
		plain := insertRow(ctx, t, conn, insertBurger, "Plain")
		other := insertRow(ctx, t, conn, insertBurger, "Other")
		mustLink := func(shopID, burgerID int64) {
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
				t.Fatalf("link shop %d burger %d: %v", shopID, burgerID, err)
			}
		}
		mustLink(deltaDiner, cheese)
		mustLink(deltaDiner, plain)
		mustLink(pctBeef, other)

		insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
		t1 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
		t2 := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
		r1 := insertRow(ctx, t, conn, insertReview, 5, "Tasty", alice, cheese, nil, t1)
		r2 := insertRow(ctx, t, conn, insertReview, 3, nil, carol, cheese, nil, t2)
		r3 := insertRow(ctx, t, conn, insertReview, 4, nil, alice, plain, nil, t2) // same time as r2: id desc breaks the tie
		insertRow(ctx, t, conn, insertReview, 1, "discarded", alice, cheese, time.Now(), t2)
		insertRow(ctx, t, conn, insertReview, 2, "other shop", alice, other, nil, t2)
		if _, err := conn.Exec(ctx,
			`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
			 VALUES ($1, 2, 4.0, 3.9, 0.7, now())`, cheese); err != nil {
			t.Fatalf("insert burger stats: %v", err)
		}

		reviews, err := repo.ListShopReviews(ctx, deltaDiner)
		if err != nil {
			t.Fatalf("ListShopReviews returned error: %v", err)
		}
		comment := "Tasty"
		want := []domain.ShopReview{
			{
				ID: r3, Rating: 4, CreatedAt: t2,
				User:   &domain.UserRef{ID: alice, Username: "alice"},
				Burger: &domain.ShopReviewBurger{ID: plain, Name: "Plain"}, // no stats row: zeros
			},
			{
				ID: r2, Rating: 3, CreatedAt: t2,
				User:   &domain.UserRef{ID: carol, Username: "carol"},
				Burger: &domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7},
			},
			{
				ID: r1, Rating: 5, Comment: &comment, CreatedAt: t1,
				User:   &domain.UserRef{ID: alice, Username: "alice"},
				Burger: &domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7},
			},
		}
		if len(reviews) != len(want) {
			t.Fatalf("got %d reviews (%+v), want %d", len(reviews), reviews, len(want))
		}
		for i := range want {
			got := reviews[i]
			// Compare timestamps by instant (driver returns them in the
			// session time zone), then align for the DeepEqual below.
			if !got.CreatedAt.Equal(want[i].CreatedAt) {
				t.Errorf("review[%d].CreatedAt = %v, want %v", i, got.CreatedAt, want[i].CreatedAt)
			}
			got.CreatedAt = want[i].CreatedAt
			if !reflect.DeepEqual(got, want[i]) {
				t.Errorf("review[%d] = %+v, want %+v", i, got, want[i])
			}
		}

		if got, err := repo.ListShopReviews(ctx, golfRejected); err != nil || len(got) != 0 {
			t.Errorf("reviews of shop without burgers = %v, %v; want empty, nil", got, err)
		}
	})
}
