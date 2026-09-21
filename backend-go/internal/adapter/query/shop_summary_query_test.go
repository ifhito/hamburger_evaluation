package query_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
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

// summaryFixture は、ショップの集計の検証に使う、ショップ・バーガー・レビューを SQL で用意する道具である。
type summaryFixture struct {
	t    *testing.T
	ctx  context.Context
	conn *pgx.Conn
	base time.Time
}

func (f summaryFixture) shop(name string) string {
	return dbtest.InsertUUIDRow(f.ctx, f.t, f.conn,
		`INSERT INTO shops (name, status) VALUES ($1, 1) RETURNING id`, name)
}

// burger は、shopID のショップに紐づくバーガーを 1 つ作る。
func (f summaryFixture) burger(shopID, name string) string {
	id := dbtest.InsertUUIDRow(f.ctx, f.t, f.conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name)
	if _, err := f.conn.Exec(f.ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, id); err != nil {
		f.t.Fatalf("link burger: %v", err)
	}
	return id
}

// review は、レビューを 1 件作る。minutes は base からの経過分(投稿の新しさ)、photoKey が空なら写真なし、
// discarded なら削除済みのレビューにする。作ったレビューの id を返す。
func (f summaryFixture) review(userID, burgerID string, rating int, minutes int, photoKey string, discarded bool) string {
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

func TestShopSummaries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	f := summaryFixture{t: t, ctx: ctx, conn: conn, base: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	shopQuery := query.NewShopQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice")
	bob := dbtest.InsertUserRow(ctx, t, conn, insertUser, "bob@example.com", "bob")
	gone := dbtest.InsertUserRow(ctx, t, conn, insertUser, "gone@example.com", "gone")
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}

	summaries := func(t *testing.T, ids ...string) map[string]domain.ShopSummary {
		t.Helper()
		got, err := shopQuery.ListShopSummaries(ctx, ids)
		if err != nil {
			t.Fatalf("ListShopSummaries returned error: %v", err)
		}
		return got
	}

	t.Run("削除済みのレビューと退会した利用者のレビューは、件数にも平均にも写真にも入らない", func(t *testing.T) {
		shop := f.shop("Counted Grill")
		b1 := f.burger(shop, "Classic")
		b2 := f.burger(shop, "Cheese") // 同じショップの別のバーガーも、まとめて数える
		f.review(alice, b1, 5, 1, "reviews/older.jpg", false)
		f.review(bob, b2, 4, 2, "reviews/newest-kept.jpg", false)
		f.review(bob, b1, 4, 3, "", false)                                 // いちばん新しいが、写真なし
		f.review(alice, b2, 1, 4, "reviews/newer-but-discarded.jpg", true) // 削除済み
		f.review(gone, b1, 1, 5, "reviews/newest-by-gone-user.jpg", false) // 退会した利用者
		got := summaries(t, shop)[shop]
		if got.ReviewCount != 3 {
			t.Errorf("ReviewCount = %d, want 3", got.ReviewCount)
		}
		if got.AverageRating == nil || *got.AverageRating != 4.3 {
			t.Errorf("AverageRating = %v, want 4.3((5+4+4)/3 を小数 1 桁に丸めた値)", got.AverageRating)
		}
		if got.PhotoKey == nil || *got.PhotoKey != "reviews/newest-kept.jpg" {
			t.Errorf("PhotoKey = %v, want reviews/newest-kept.jpg(写真つきで最も新しい、削除されていないレビューの写真)", got.PhotoKey)
		}
	})

	t.Run("写真つきのレビューが同じ時刻に 2 件あるときは、id が新しい(大きい)方の写真を使う", func(t *testing.T) {
		shop := f.shop("Tie Diner")
		b := f.burger(shop, "Classic")
		id1 := f.review(alice, b, 4, 7, "reviews/tie-1.jpg", false)
		id2 := f.review(bob, b, 4, 7, "reviews/tie-2.jpg", false)
		want := map[string]string{id1: "reviews/tie-1.jpg", id2: "reviews/tie-2.jpg"}[max(id1, id2)]
		got := summaries(t, shop)[shop]
		if got.PhotoKey == nil || *got.PhotoKey != want {
			t.Errorf("PhotoKey = %v, want %q", got.PhotoKey, want)
		}
	})

	t.Run("写真つきのレビューがないショップは、写真が nil で、件数と平均だけがある", func(t *testing.T) {
		shop := f.shop("No Photo Cafe")
		b := f.burger(shop, "Plain")
		f.review(alice, b, 3, 8, "", false)
		f.review(bob, b, 4, 9, "", false)
		got := summaries(t, shop)[shop]
		if got.PhotoKey != nil || got.ReviewCount != 2 || got.AverageRating == nil || *got.AverageRating != 3.5 {
			t.Errorf("summary = %+v, want count 2, average 3.5, no photo", got)
		}
	})

	t.Run("レビューのないショップは、結果に含まれない(呼び出し側が、空の集計にする)", func(t *testing.T) {
		shop := f.shop("Empty Kitchen")
		f.burger(shop, "Unreviewed")
		if got, ok := summaries(t, shop)[shop]; ok {
			t.Errorf("summary = %+v, want no entry", got)
		}
	})

	t.Run("平均の小数 2 桁目がちょうど 5 のときは、切り上げる", func(t *testing.T) {
		shop := f.shop("Round Half")
		b := f.burger(shop, "Classic")
		for i, rating := range []int{4, 4, 4, 5} {
			f.review(alice, b, rating, 10+i, "", false)
		}
		got := summaries(t, shop)[shop]
		if got.AverageRating == nil || *got.AverageRating != 4.3 {
			t.Errorf("AverageRating = %v, want 4.3((4+4+4+5)/4 = 4.25 を切り上げた値)", got.AverageRating)
		}
	})

	t.Run("ほかのショップのレビューは、混ざらない", func(t *testing.T) {
		mine := f.shop("Mine")
		other := f.shop("Other")
		f.review(alice, f.burger(mine, "Classic"), 5, 20, "reviews/mine.jpg", false)
		f.review(bob, f.burger(other, "Classic"), 1, 21, "reviews/other-newer.jpg", false)
		got := summaries(t, mine, other)
		if got[mine].ReviewCount != 1 || *got[mine].PhotoKey != "reviews/mine.jpg" || *got[mine].AverageRating != 5 {
			t.Errorf("mine = %+v", got[mine])
		}
		if got[other].ReviewCount != 1 || *got[other].PhotoKey != "reviews/other-newer.jpg" || *got[other].AverageRating != 1 {
			t.Errorf("other = %+v", got[other])
		}
	})

	t.Run("id を渡さないときは、何も問い合わせずに空の結果を返す", func(t *testing.T) {
		if got := summaries(t); len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("一覧の取得は、ショップの件数に関わらず、2 回のクエリで済む(件数に比例しない)", func(t *testing.T) {
		names := make([]string, 0, 20)
		for i := 0; i < 20; i++ {
			shop := f.shop("List Shop " + string(rune('A'+i)))
			names = append(names, "List Shop "+string(rune('A'+i)))
			b := f.burger(shop, "Classic")
			f.review(alice, b, 1+i%5, 30+i, "reviews/list-"+string(rune('a'+i))+".jpg", false)
		}
		sort.Strings(names)
		countQueries := func(keyword string, perPage int) (queries, shops int) {
			n := 0
			shopsUC := usecase.NewShops(query.NewShopQuery(countingDB{conn: conn, n: &n}), domain.NewShops(nil))
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
		if twenty != 2 || one != 2 {
			t.Errorf("クエリの回数 = 20 件で %d 回・1 件で %d 回, want どちらも 2 回(ショップの一覧 1 回 + 集計 1 回)", twenty, one)
		}
	})
}
