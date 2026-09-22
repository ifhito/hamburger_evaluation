package query_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// countingDB は、DB に送ったクエリの回数を数える(N+1 でないことを示すため)。
type countingDB struct {
	conn *pgx.Conn
	n    *int
}

func (c countingDB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	*c.n++
	return c.conn.Exec(ctx, sql, args...)
}

func (c countingDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	*c.n++
	return c.conn.Query(ctx, sql, args...)
}

func (c countingDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	*c.n++
	return c.conn.QueryRow(ctx, sql, args...)
}

// statsFixture は、ショップの集計の検証に使う、ショップ・バーガー・レビュー・保存された集計を SQL で用意する道具である。
type statsFixture struct {
	t    *testing.T
	ctx  context.Context
	conn *pgx.Conn
	base time.Time
}

func (f statsFixture) shop(name string) string {
	return dbtest.InsertUUIDRow(f.ctx, f.t, f.conn,
		`INSERT INTO shops (name, status) VALUES ($1, 1) RETURNING id`, name)
}

// burger は、shopIDs のショップに紐づくバーガーを 1 つ作る。
func (f statsFixture) burger(name string, shopIDs ...string) string {
	id := dbtest.InsertUUIDRow(f.ctx, f.t, f.conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name)
	for _, shopID := range shopIDs {
		if _, err := f.conn.Exec(f.ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, id); err != nil {
			f.t.Fatalf("link burger: %v", err)
		}
	}
	return id
}

// review は、レビューを 1 件作る。minutes は base からの経過分(投稿の新しさ)、photoKey が空なら写真なし、
// discarded なら削除済みのレビューにする。作ったレビューの id を返す。
func (f statsFixture) review(userID, burgerID string, rating int, minutes int, photoKey string, discarded bool) string {
	var photo, discardedAt any
	if photoKey != "" {
		photo = photoKey
	}
	created := f.base.Add(time.Duration(minutes) * time.Minute)
	if discarded {
		discardedAt = created
	}
	return dbtest.InsertUUIDRow(f.ctx, f.t, f.conn,
		`INSERT INTO reviews (rating, user_id, burger_id, photo_key, discarded_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		rating, userID, burgerID, photo, discardedAt, created)
}

// stat は、ショップの保存された集計(shop_stats の行)を直接作る。average が nil なら、レビューなしの集計(件数 0)。
func (f statsFixture) stat(shopID string, count int, average *float64, photo *string) {
	f.t.Helper()
	if _, err := f.conn.Exec(f.ctx,
		`INSERT INTO shop_stats (shop_id, review_count, average_rating, photo_key, calculated_at) VALUES ($1, $2, $3, $4, now())`,
		shopID, count, average, photo); err != nil {
		f.t.Fatalf("insert shop stat: %v", err)
	}
}

func strPtr(s string) *string { return &s }

func floatPtr(f float64) *float64 { return &f }

// TestShopQueryReadsStoredStats は、ショップの一覧・詳細が、保存された集計(shop_stats)を LEFT JOIN で添えて返し、
// レビューから直接は集計しないこと(保存された値をそのまま返すこと)、まだ集計されていないショップは空の集計になること、
// クエリがショップの件数に比例しないことを確かめる。
func TestShopQueryReadsStoredStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	f := statsFixture{t: t, ctx: ctx, conn: conn, base: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	shopQuery := query.NewShopQuery(conn)
	anon := domain.ShopVisibilityFor(nil)

	insertUser := `INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice")

	counted := f.shop("A Counted")
	unstated := f.shop("B Unstated") // 集計の行がない(まだ集計されていない)
	empty := f.shop("C Empty")       // レビューなしの集計(件数 0)
	f.stat(counted, 3, floatPtr(4.3), strPtr("reviews/latest.jpg"))
	f.stat(empty, 0, nil, nil)
	// 保存された集計は、レビューの実際の内容とは無関係に、そのまま返される(レビューから直接は集計しない)。
	b := f.burger("Classic", counted)
	f.review(alice, b, 1, 1, "reviews/never-used.jpg", false)

	listings := func(t *testing.T) map[string]domain.ShopSummary {
		t.Helper()
		got, _, err := shopQuery.ListShops(ctx, anon, "", 100, 0)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		out := map[string]domain.ShopSummary{}
		for _, l := range got {
			out[l.ID] = l.Summary
		}
		return out
	}

	t.Run("一覧は、保存された集計(件数・平均・写真のキー)を、そのまま返す", func(t *testing.T) {
		got := listings(t)[counted]
		want := domain.ShopSummary{ReviewCount: 3, AverageRating: floatPtr(4.3), PhotoKey: strPtr("reviews/latest.jpg")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("summary = %+v, want %+v", got, want)
		}
	})

	t.Run("まだ集計されていないショップ(集計の行がない)は、件数 0・平均と写真なしの空の集計になる", func(t *testing.T) {
		if got := listings(t)[unstated]; !reflect.DeepEqual(got, domain.ShopSummary{}) {
			t.Errorf("summary = %+v, want 空", got)
		}
	})

	t.Run("レビューなしの集計(件数 0)も、件数 0・平均と写真なしとして返る", func(t *testing.T) {
		if got := listings(t)[empty]; !reflect.DeepEqual(got, domain.ShopSummary{}) {
			t.Errorf("summary = %+v, want 空", got)
		}
	})

	t.Run("詳細も、保存された集計を返す(集計のないショップは空の集計)", func(t *testing.T) {
		detail, err := shopQuery.GetShopWithCreator(ctx, counted)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if want := (domain.ShopSummary{ReviewCount: 3, AverageRating: floatPtr(4.3), PhotoKey: strPtr("reviews/latest.jpg")}); !reflect.DeepEqual(detail.Summary, want) {
			t.Errorf("summary = %+v, want %+v", detail.Summary, want)
		}
		detail, err = shopQuery.GetShopWithCreator(ctx, unstated)
		if err != nil || !reflect.DeepEqual(detail.Summary, domain.ShopSummary{}) {
			t.Errorf("summary = %+v (err %v), want 空", detail.Summary, err)
		}
	})

	t.Run("一覧の取得は、ショップの件数に関わらず、同じ回数(1 回)のクエリで済む(件数に比例しない)", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			shop := f.shop("List Shop " + string(rune('A'+i)))
			f.stat(shop, i+1, floatPtr(3.5), nil)
		}
		countQueries := func(keyword string, perPage int) (queries, shops int) {
			n := 0
			shopsUC := usecase.NewShops(query.NewShopQuery(countingDB{conn: conn, n: &n}), domain.NewShops(nil), storage.NewDisk("", "/photos"))
			list, _, err := shopsUC.List(ctx, nil, keyword, 1, perPage)
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			return n, len(list)
		}
		twenty, gotShops := countQueries("List Shop", 20)
		if gotShops != 20 {
			t.Fatalf("shops = %d, want 20", gotShops)
		}
		one, gotOne := countQueries("List Shop A", 20)
		if gotOne != 1 {
			t.Fatalf("shops = %d, want 1", gotOne)
		}
		if twenty != one || twenty != 1 {
			t.Errorf("クエリの回数 = 20 件で %d 回・1 件で %d 回, want どちらも 1 回(ショップの件数に比例しない。集計は LEFT JOIN で添える)", twenty, one)
		}
	})
}

// TestShopStatsQuery は、ショップの集計の元データ(ショップのすべてのバーガーのレビュー。削除済み・退会した利用者の
// レビューを除く)と、投稿者の履歴、バーガーが紐づくショップ、再計算の依頼の一覧の読み取りを確かめる。
func TestShopStatsQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	f := statsFixture{t: t, ctx: ctx, conn: conn, base: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	statsQuery := query.NewShopStatsQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice")
	bob := dbtest.InsertUserRow(ctx, t, conn, insertUser, "bob@example.com", "bob")
	gone := dbtest.InsertUserRow(ctx, t, conn, insertUser, "gone@example.com", "gone")
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}

	t.Run("ショップのすべてのバーガーのレビューを、削除済み・退会した利用者のレビューを除いて、投稿の古い順に返す", func(t *testing.T) {
		shop, other := f.shop("Facts Grill"), f.shop("Other Grill")
		classic, cheese, elsewhere := f.burger("Classic", shop), f.burger("Cheese", shop), f.burger("Elsewhere", other)
		first := f.review(alice, classic, 5, 1, "reviews/first.jpg", false)
		second := f.review(bob, cheese, 3, 2, "", false)
		f.review(alice, cheese, 1, 3, "reviews/discarded.jpg", true) // 削除済み
		f.review(gone, classic, 1, 4, "reviews/gone.jpg", false)     // 退会した利用者
		f.review(bob, elsewhere, 2, 5, "", false)                    // 別のショップ
		facts, err := statsQuery.ListShopReviewFacts(ctx, shop)
		if err != nil {
			t.Fatalf("ListShopReviewFacts returned error: %v", err)
		}
		if len(facts) != 2 || facts[0].ID != first || facts[1].ID != second {
			t.Fatalf("facts = %+v, want レビュー [%s %s](複数のバーガーをまとめ、古い順)", facts, first, second)
		}
		if facts[0].Rating != 5 || facts[0].PhotoKey == nil || *facts[0].PhotoKey != "reviews/first.jpg" || facts[1].PhotoKey != nil {
			t.Errorf("facts = %+v, want 評価と写真のキー(写真なしは nil)", facts)
		}
	})

	t.Run("それぞれのレビューに、投稿者が、すべてのバーガー(ほかのショップも含む)に付けた有効な評価を、履歴として添える", func(t *testing.T) {
		shop, other := f.shop("History Grill"), f.shop("History Other")
		here, there := f.burger("Here", shop), f.burger("There", other)
		reviewer := dbtest.InsertUserRow(ctx, t, conn, insertUser, "history@example.com", "history") // ほかのレビューを持たない投稿者
		f.review(reviewer, here, 5, 10, "", false)
		f.review(reviewer, there, 3, 11, "", false)                     // 別のショップのレビューも、履歴に入る
		f.review(reviewer, there, 1, 12, "reviews/discarded.jpg", true) // 削除済みは、履歴に入らない
		facts, err := statsQuery.ListShopReviewFacts(ctx, shop)
		if err != nil {
			t.Fatalf("ListShopReviewFacts returned error: %v", err)
		}
		if len(facts) != 1 {
			t.Fatalf("facts = %+v, want 1 件", facts)
		}
		got := append([]float64(nil), facts[0].ReviewerHistory.Ratings...)
		if !reflect.DeepEqual(sortedFloats(got), []float64{3, 5}) {
			t.Errorf("履歴 = %v, want 投稿者の有効な評価 [3 5](このショップのレビューと、別のショップのレビュー)", got)
		}
	})

	t.Run("レビューのないショップは、空の一覧を返す", func(t *testing.T) {
		facts, err := statsQuery.ListShopReviewFacts(ctx, f.shop("No Reviews Grill"))
		if err != nil || len(facts) != 0 {
			t.Errorf("facts = %+v (err %v), want 空", facts, err)
		}
	})

	t.Run("バーガーが紐づくショップの id を、重複なしで昇順に返す(紐づかないバーガーは空)", func(t *testing.T) {
		s1, s2 := f.shop("Burger Shop 1"), f.shop("Burger Shop 2")
		burger := f.burger("Shared", s2, s1)
		got, err := statsQuery.ListBurgerShopIDs(ctx, burger)
		if err != nil {
			t.Fatalf("ListBurgerShopIDs returned error: %v", err)
		}
		want := []string{s1, s2}
		if want[0] > want[1] {
			want[0], want[1] = want[1], want[0]
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("shop ids = %v, want %v(昇順)", got, want)
		}
		if got, err := statsQuery.ListBurgerShopIDs(ctx, f.burger("Orphan")); err != nil || len(got) != 0 {
			t.Errorf("紐づかないバーガー: %v (err %v), want 空", got, err)
		}
	})

	t.Run("再計算の時期が来ている依頼だけを、失敗の回数の上限と件数の上限の範囲で返す", func(t *testing.T) {
		now := time.Now().Truncate(time.Microsecond)
		due, waiting, exhausted := f.shop("Due Shop"), f.shop("Waiting Shop"), f.shop("Exhausted Shop")
		for _, shop := range []string{due, waiting, exhausted} {
			if _, err := conn.Exec(ctx, `INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)`, shop); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(ctx, `UPDATE shop_stats_recalc_requests SET attempts = 1, next_attempt_at = $2 WHERE shop_id = $1`, waiting, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `UPDATE shop_stats_recalc_requests SET attempts = 5 WHERE shop_id = $1`, exhausted); err != nil {
			t.Fatal(err)
		}
		requests, err := statsQuery.ListDueShopRecalcRequests(ctx, now, 5, 100)
		if err != nil {
			t.Fatalf("ListDueShopRecalcRequests returned error: %v", err)
		}
		var got []string
		for _, r := range requests {
			got = append(got, r.ShopID)
		}
		if !reflect.DeepEqual(got, []string{due}) {
			t.Errorf("due = %v, want [%s](待ち時間の中・失敗の上限に達したものは含まない)", got, due)
		}
		if got := requests[0]; got.Version <= 0 || got.Attempts != 0 {
			t.Errorf("request = %+v, want version と attempts が入っている", got)
		}
	})
}

func sortedFloats(in []float64) []float64 {
	out := append([]float64(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
