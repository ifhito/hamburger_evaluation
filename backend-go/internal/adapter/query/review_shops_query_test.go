package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

func TestReviewQueryListReviewShops(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)

	alice := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`, "alice@example.com", "alice")
	// id を明示して作る(並びが、id ではなく、作成の古い順で決まることを示すため)。
	shopWith := func(id, name string, status int, creator any, created time.Time) string {
		t.Helper()
		return dbtest.InsertUUIDRow(ctx, t, conn,
			`INSERT INTO shops (id, name, status, creator_id, created_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`, id, name, status, creator, created)
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
	shops := func(t *testing.T, reviewID string) []domain.Shop {
		t.Helper()
		got, err := reviewQuery.ListReviewShops(ctx, reviewID)
		if err != nil {
			t.Fatalf("ListReviewShops returned error: %v", err)
		}
		return got
	}
	ids := func(list []domain.Shop) []string {
		out := make([]string, 0, len(list))
		for _, shop := range list {
			out = append(out, shop.ID)
		}
		return out
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("レビューのバーガーを持つショップを、状態(承認済み・承認待ち・却下)と作成者とともに返す", func(t *testing.T) {
		active := shopWith("00000000-0000-4000-8000-000000000101", "Active", 1, nil, base)
		pending := shopWith("00000000-0000-4000-8000-000000000102", "Pending", 0, alice, base)
		rejected := shopWith("00000000-0000-4000-8000-000000000103", "Rejected", 2, nil, base)
		for name, tt := range map[string]struct {
			shopID string
			status domain.ShopStatus
		}{"active": {active, domain.ShopStatusActive}, "pending": {pending, domain.ShopStatusPending}, "rejected": {rejected, domain.ShopStatusRejected}} {
			got := shops(t, reviewOf(burgerIn("Burger "+name, tt.shopID)))
			if len(got) != 1 || got[0].ID != tt.shopID || got[0].Status != tt.status {
				t.Errorf("%s: shops = %+v, want 1 件で id %s status %s", name, got, tt.shopID, tt.status)
			}
		}
		got := shops(t, reviewOf(burgerIn("Burger pending 2", pending)))
		if len(got) != 1 || got[0].CreatorID == nil || *got[0].CreatorID != alice || got[0].Name != "Pending" {
			t.Errorf("承認待ちのショップ = %+v, want 名前と作成者 %s(can_review の判定に使う)", got, alice)
		}
	})

	t.Run("バーガーが複数のショップにあるときは、すべてを、作成の古い順(同時刻は id 順)に返す", func(t *testing.T) {
		newest := shopWith("00000000-0000-4000-8000-000000000201", "Newest", 1, nil, base.Add(time.Hour)) // id は最小だが、最も新しい
		olderB := shopWith("00000000-0000-4000-8000-000000000203", "OlderB", 2, nil, base)                // 古い。同時刻の 2 件のうち id が大きい方
		olderA := shopWith("00000000-0000-4000-8000-000000000202", "OlderA", 1, nil, base)                // 古い。同時刻の 2 件のうち id が小さい方
		got := ids(shops(t, reviewOf(burgerIn("Shared", newest, olderB, olderA))))
		want := []string{olderA, olderB, newest}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("shops = %v, want %v", got, want)
		}
	})

	t.Run("ショップに紐づかないバーガーのレビューは、空の一覧を返し、エラーにならない", func(t *testing.T) {
		if got := shops(t, reviewOf(burgerIn("Orphan"))); len(got) != 0 {
			t.Errorf("shops = %+v, want 空", got)
		}
	})

	t.Run("削除済みのレビューは、空の一覧を返す", func(t *testing.T) {
		shop := shopWith("00000000-0000-4000-8000-000000000301", "Gone", 1, nil, base)
		id := reviewOf(burgerIn("Gone burger", shop))
		if _, err := conn.Exec(ctx, `UPDATE reviews SET discarded_at = now() WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if got := shops(t, id); len(got) != 0 {
			t.Errorf("shops = %+v, want 空", got)
		}
	})

	t.Run("存在しないレビューは、空の一覧を返し、エラーにならない", func(t *testing.T) {
		if got := shops(t, "00000000-0000-4000-8000-0000000000ff"); len(got) != 0 {
			t.Errorf("shops = %+v, want 空", got)
		}
	})
}
