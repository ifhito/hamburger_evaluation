package query_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

func TestReviewQueryGetReviewShop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)

	alice := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`, "alice@example.com", "alice")
	shopAt := func(name string, status int, creator any, created time.Time) string {
		t.Helper()
		return dbtest.InsertUUIDRow(ctx, t, conn,
			`INSERT INTO shops (name, status, creator_id, created_at) VALUES ($1, $2, $3, $4) RETURNING id`, name, status, creator, created)
	}
	burgerIn := func(name string, shopIDs ...string) string {
		t.Helper()
		id := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name)
		for _, shopID := range shopIDs {
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, id); err != nil {
				t.Fatalf("link burger: %v", err)
			}
		}
		return id
	}
	reviewOf := func(burgerID string) string {
		t.Helper()
		return dbtest.InsertUUIDRow(ctx, t, conn,
			`INSERT INTO reviews (rating, user_id, burger_id) VALUES (4, $1, $2) RETURNING id`, alice, burgerID)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("レビューのバーガーを持つショップを、状態(承認済み・承認待ち・却下)とともに返す", func(t *testing.T) {
		active := shopAt("Active", 1, nil, base)
		pending := shopAt("Pending", 0, alice, base)
		rejected := shopAt("Rejected", 2, nil, base)
		for name, tt := range map[string]struct {
			shopID string
			status domain.ShopStatus
		}{"active": {active, domain.ShopStatusActive}, "pending": {pending, domain.ShopStatusPending}, "rejected": {rejected, domain.ShopStatusRejected}} {
			got, err := reviewQuery.GetReviewShop(ctx, reviewOf(burgerIn("Burger "+name, tt.shopID)))
			if err != nil {
				t.Fatalf("%s: GetReviewShop returned error: %v", name, err)
			}
			if got.ID != tt.shopID || got.Status != tt.status {
				t.Errorf("%s: shop = %+v, want id %s status %s", name, got, tt.shopID, tt.status)
			}
		}
		got, err := reviewQuery.GetReviewShop(ctx, reviewOf(burgerIn("Burger pending 2", pending)))
		if err != nil || got.CreatorID == nil || *got.CreatorID != alice {
			t.Errorf("承認待ちのショップの作成者 = %+v (err %v), want %s(can_review の判定に使う)", got.CreatorID, err, alice)
		}
	})

	t.Run("バーガーが複数のショップにあるときは、作成の古い方のショップを返す", func(t *testing.T) {
		newer := shopAt("Newer", 1, nil, base.Add(time.Hour)) // 先に作って、id の順・挿入の順では決まらないことを示す
		older := shopAt("Older", 1, nil, base)
		got, err := reviewQuery.GetReviewShop(ctx, reviewOf(burgerIn("Shared", newer, older)))
		if err != nil {
			t.Fatalf("GetReviewShop returned error: %v", err)
		}
		if got.ID != older {
			t.Errorf("shop = %s, want %s(作成の古い方)", got.ID, older)
		}
	})

	t.Run("存在しないレビューは、ErrReviewNotFound を返す", func(t *testing.T) {
		_, err := reviewQuery.GetReviewShop(ctx, "00000000-0000-4000-8000-0000000000ff")
		if !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("err = %v, want ErrReviewNotFound", err)
		}
	})
}
