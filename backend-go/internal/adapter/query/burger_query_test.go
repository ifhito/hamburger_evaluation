package query_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// このファイルは adapter/query の DB 統合テストである。burger 1 件の読み取り
// (BurgerQuery.GetBurgerWithStats・ListBurgerShops)と、GET /burgers 向けの読み取り
// (BurgerQuery.ListBurgerRankings)の並び順・除外・代表ショップの選び方・pagination を、
// 実 PostgreSQL で検証する。1 テストにつき 1 つの新しい database を使い、他のテストの
// fixture と混じらないようにする。

// insertShopAt は、指定した created_at を持つ shop を作り、id を返す。
// status のコード：0=pending、1=active、2=rejected。
func insertShopAt(ctx context.Context, t *testing.T, conn *pgx.Conn, name string, status int16, createdAt time.Time) string {
	t.Helper()
	return dbtest.InsertUUIDRow(ctx, t, conn,
		`INSERT INTO shops (name, status, created_at) VALUES ($1, $2, $3) RETURNING id`, name, status, createdAt)
}

// insertBurger は burger を作り、id を返す。
func insertBurger(ctx context.Context, t *testing.T, conn *pgx.Conn, name string) string {
	t.Helper()
	return dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name)
}

// linkShopBurger は shops_burgers に 1 行足す。
func linkShopBurger(ctx context.Context, t *testing.T, conn *pgx.Conn, shopID, burgerID string) {
	t.Helper()
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
		t.Fatalf("link shop %s burger %s: %v", shopID, burgerID, err)
	}
}

// insertBurgerStats は burger_stats に 1 行足す(review_count・confidence は、この一覧のテストでは
// 意味を持たないので固定値)。
func insertBurgerStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID string, weightedScore float64) {
	t.Helper()
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 3, 4.0, $2, 0.5, now())`, burgerID, weightedScore); err != nil {
		t.Fatalf("insert burger stats for %s: %v", burgerID, err)
	}
}

func TestBurgerQueryOrdersByWeightedScoreDescending(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	shop := insertShopAt(ctx, t, conn, "Order Shop", 1, time.Now())
	low := insertBurger(ctx, t, conn, "Low Score")
	mid := insertBurger(ctx, t, conn, "Mid Score")
	high := insertBurger(ctx, t, conn, "High Score")
	for _, id := range []string{low, mid, high} {
		linkShopBurger(ctx, t, conn, shop, id)
	}
	insertBurgerStats(ctx, t, conn, low, 1.0)
	insertBurgerStats(ctx, t, conn, mid, 2.0)
	insertBurgerStats(ctx, t, conn, high, 3.0)

	rankings, hasMore, err := burgerQuery.ListBurgerRankings(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListBurgerRankings returned error: %v", err)
	}
	if hasMore {
		t.Errorf("hasMore = true, want false")
	}
	if len(rankings) != 3 {
		t.Fatalf("got %d rankings, want 3: %+v", len(rankings), rankings)
	}
	want := []string{high, mid, low}
	for i, id := range want {
		if rankings[i].ID != id {
			t.Errorf("rankings[%d].ID = %s, want %s (weighted_score desc)", i, rankings[i].ID, id)
		}
	}
}

func TestBurgerQueryExcludesBurgerWithoutStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	shop := insertShopAt(ctx, t, conn, "Shop", 1, time.Now())
	withStats := insertBurger(ctx, t, conn, "With Stats")
	withoutStats := insertBurger(ctx, t, conn, "Without Stats")
	stale := insertBurger(ctx, t, conn, "Stale") // 統計行はあるが 0 件に戻った
	linkShopBurger(ctx, t, conn, shop, withStats)
	linkShopBurger(ctx, t, conn, shop, withoutStats)
	linkShopBurger(ctx, t, conn, shop, stale)
	insertBurgerStats(ctx, t, conn, withStats, 1.0)
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 0, 0, 0, 0, now())`, stale); err != nil {
		t.Fatalf("insert stale burger stats: %v", err)
	}

	rankings, _, err := burgerQuery.ListBurgerRankings(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListBurgerRankings returned error: %v", err)
	}
	if len(rankings) != 1 || rankings[0].ID != withStats {
		t.Errorf("rankings = %+v, want only %s (review が無い burger と、削除で 0 件に戻った burger は対象外)", rankings, withStats)
	}
}

func TestBurgerQueryRepresentativeShopIsOldestActive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	older := insertShopAt(ctx, t, conn, "Older Active Shop", 1, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	newer := insertShopAt(ctx, t, conn, "Newer Active Shop", 1, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	burger := insertBurger(ctx, t, conn, "Shared Burger")
	// 先に newer を結び付け、DISTINCT ON が created_at 順で選ぶこと(挿入順ではない)を確かめる。
	linkShopBurger(ctx, t, conn, newer, burger)
	linkShopBurger(ctx, t, conn, older, burger)
	insertBurgerStats(ctx, t, conn, burger, 1.0)

	rankings, _, err := burgerQuery.ListBurgerRankings(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListBurgerRankings returned error: %v", err)
	}
	if len(rankings) != 1 {
		t.Fatalf("got %d rankings, want 1: %+v", len(rankings), rankings)
	}
	want := domain.ShopRef{ID: older, Name: "Older Active Shop"}
	if rankings[0].Shop != want {
		t.Errorf("shop = %+v, want %+v (作成が最も古い active な shop)", rankings[0].Shop, want)
	}
}

func TestBurgerQueryRepresentativeShopSkipsPendingShop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	// pending の方を先に(より古く)作っても、active な shop だけが代表に選ばれる。
	pending := insertShopAt(ctx, t, conn, "Pending Shop", 0, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	active := insertShopAt(ctx, t, conn, "Active Shop", 1, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	burger := insertBurger(ctx, t, conn, "Burger")
	linkShopBurger(ctx, t, conn, pending, burger)
	linkShopBurger(ctx, t, conn, active, burger)
	insertBurgerStats(ctx, t, conn, burger, 1.0)

	rankings, _, err := burgerQuery.ListBurgerRankings(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListBurgerRankings returned error: %v", err)
	}
	if len(rankings) != 1 {
		t.Fatalf("got %d rankings, want 1: %+v", len(rankings), rankings)
	}
	want := domain.ShopRef{ID: active, Name: "Active Shop"}
	if rankings[0].Shop != want {
		t.Errorf("shop = %+v, want %+v (pending な shop は代表に選ばれない)", rankings[0].Shop, want)
	}
}

func TestBurgerQueryExcludesBurgerWithoutActiveShop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	pending := insertShopAt(ctx, t, conn, "Pending Shop", 0, time.Now())
	rejected := insertShopAt(ctx, t, conn, "Rejected Shop", 2, time.Now())
	burger := insertBurger(ctx, t, conn, "Not Yet Approved Burger")
	linkShopBurger(ctx, t, conn, pending, burger)
	linkShopBurger(ctx, t, conn, rejected, burger)
	// creator や admin によるレビューで、承認前でも burger_stats が計算されていることがある。
	insertBurgerStats(ctx, t, conn, burger, 1.0)

	rankings, _, err := burgerQuery.ListBurgerRankings(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListBurgerRankings returned error: %v", err)
	}
	if len(rankings) != 0 {
		t.Errorf("rankings = %+v, want empty (active な shop が 1 つもない burger は対象外)", rankings)
	}
}

func TestBurgerQueryPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	shop := insertShopAt(ctx, t, conn, "Shop", 1, time.Now())
	scores := []float64{3.0, 2.0, 1.0}
	for _, score := range scores {
		burger := insertBurger(ctx, t, conn, "Burger")
		linkShopBurger(ctx, t, conn, shop, burger)
		insertBurgerStats(ctx, t, conn, burger, score)
	}

	t.Run("1 ページ目は limit 件を返し、続きがあれば has_more は true", func(t *testing.T) {
		rankings, hasMore, err := burgerQuery.ListBurgerRankings(ctx, 2, 0)
		if err != nil {
			t.Fatalf("ListBurgerRankings returned error: %v", err)
		}
		if len(rankings) != 2 || !hasMore {
			t.Errorf("len = %d, hasMore = %v, want 2, true", len(rankings), hasMore)
		}
	})

	t.Run("最後のページは残りの件数を返し、has_more は false", func(t *testing.T) {
		rankings, hasMore, err := burgerQuery.ListBurgerRankings(ctx, 2, 2)
		if err != nil {
			t.Fatalf("ListBurgerRankings returned error: %v", err)
		}
		if len(rankings) != 1 || hasMore {
			t.Errorf("len = %d, hasMore = %v, want 1, false", len(rankings), hasMore)
		}
	})

	t.Run("件数ちょうどの limit は has_more が false になる", func(t *testing.T) {
		rankings, hasMore, err := burgerQuery.ListBurgerRankings(ctx, 3, 0)
		if err != nil {
			t.Fatalf("ListBurgerRankings returned error: %v", err)
		}
		if len(rankings) != 3 || hasMore {
			t.Errorf("len = %d, hasMore = %v, want 3, false", len(rankings), hasMore)
		}
	})
}

func TestBurgerQueryTieBreaksByIDAscending(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	shop := insertShopAt(ctx, t, conn, "Shop", 1, time.Now())
	a := insertBurger(ctx, t, conn, "Burger A")
	b := insertBurger(ctx, t, conn, "Burger B")
	linkShopBurger(ctx, t, conn, shop, a)
	linkShopBurger(ctx, t, conn, shop, b)
	insertBurgerStats(ctx, t, conn, a, 2.0)
	insertBurgerStats(ctx, t, conn, b, 2.0)

	rankings, _, err := burgerQuery.ListBurgerRankings(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListBurgerRankings returned error: %v", err)
	}
	if len(rankings) != 2 {
		t.Fatalf("got %d rankings, want 2: %+v", len(rankings), rankings)
	}
	first, second := a, b
	if b < a {
		first, second = b, a
	}
	if rankings[0].ID != first || rankings[1].ID != second {
		t.Errorf("ids = %s,%s, want %s,%s (weighted_score が同値なら id 昇順)", rankings[0].ID, rankings[1].ID, first, second)
	}
}

// TestBurgerQuery は、burger 詳細の読み取り（adapter/query の BurgerQuery）を、共有の dbtest の
// スキャフォールドを通じて実際の PostgreSQL に対して検証する（TEST_DATABASE_URL がなければスキップする）。
func TestBurgerQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Cheese")
	veggie := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Veggie") // 統計行なし
	stale := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Stale")   // 統計行はあるが 0 件に戻った

	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 3, 4.2, 4.0, 0.8, now())`, cheese); err != nil {
		t.Fatalf("insert burger stats: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 0, 0, 0, 0, now())`, stale); err != nil {
		t.Fatalf("insert stale burger stats: %v", err)
	}

	t.Run("GetBurgerWithStats は保存された統計をそのまま返す", func(t *testing.T) {
		detail, err := burgerQuery.GetBurgerWithStats(ctx, cheese)
		if err != nil {
			t.Fatalf("GetBurgerWithStats returned error: %v", err)
		}
		count, avg, weighted := int64(3), 4.2, 4.0
		want := domain.BurgerDetail{ID: cheese, Name: "Cheese", ReviewCount: &count, AverageRating: &avg, WeightedScore: &weighted}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("detail = %+v, want %+v", detail, want)
		}
	})

	t.Run("GetBurgerWithStats は統計行がまだない burger の統計を nil にする", func(t *testing.T) {
		detail, err := burgerQuery.GetBurgerWithStats(ctx, veggie)
		if err != nil {
			t.Fatalf("GetBurgerWithStats returned error: %v", err)
		}
		want := domain.BurgerDetail{ID: veggie, Name: "Veggie"}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("detail = %+v, want %+v (nil stats)", detail, want)
		}
	})

	t.Run("GetBurgerWithStats は削除で 0 件に戻った burger の統計も nil にする(実際の値 0 と区別する)", func(t *testing.T) {
		detail, err := burgerQuery.GetBurgerWithStats(ctx, stale)
		if err != nil {
			t.Fatalf("GetBurgerWithStats returned error: %v", err)
		}
		want := domain.BurgerDetail{ID: stale, Name: "Stale"}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("detail = %+v, want %+v (nil stats)", detail, want)
		}
	})

	t.Run("GetBurgerWithStats は存在しない id に ErrBurgerNotFound を返す", func(t *testing.T) {
		if _, err := burgerQuery.GetBurgerWithStats(ctx, uid.N(9999)); !errors.Is(err, domain.ErrBurgerNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrBurgerNotFound)
		}
	})

	t.Run("ListBurgerShops は紐づく shop を作成の古い順で返す", func(t *testing.T) {
		insertShop := `INSERT INTO shops (name, status, created_at) VALUES ($1, $2, $3) RETURNING id`
		tOld := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Old Diner", 1, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		tNew := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "New Diner", 1, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
		mustLink := func(shopID string) {
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, cheese); err != nil {
				t.Fatalf("link shop %s burger %s: %v", shopID, cheese, err)
			}
		}
		mustLink(tNew)
		mustLink(tOld)

		shops, err := burgerQuery.ListBurgerShops(ctx, cheese)
		if err != nil {
			t.Fatalf("ListBurgerShops returned error: %v", err)
		}
		want := []domain.Shop{
			{ID: tOld, Name: "Old Diner", Status: domain.ShopStatusActive},
			{ID: tNew, Name: "New Diner", Status: domain.ShopStatusActive},
		}
		if !reflect.DeepEqual(shops, want) {
			t.Errorf("shops = %+v, want %+v", shops, want)
		}
	})

	t.Run("ListBurgerShops は紐づく shop がなければ空を返す", func(t *testing.T) {
		shops, err := burgerQuery.ListBurgerShops(ctx, veggie)
		if err != nil {
			t.Fatalf("ListBurgerShops returned error: %v", err)
		}
		if len(shops) != 0 {
			t.Errorf("shops = %+v, want empty", shops)
		}
	})
}
