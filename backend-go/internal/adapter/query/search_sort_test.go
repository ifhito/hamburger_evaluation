package query_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

func TestBurgerSearchAndSort(t *testing.T) {
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	public := insertShopAt(ctx, t, conn, "公開", 1, time.Now())
	hidden := insertShopAt(ctx, t, conn, "非公開", 0, time.Now())
	a := insertBurger(ctx, t, conn, "A 100%")
	b := insertBurger(ctx, t, conn, "B 100x")
	c := insertBurger(ctx, t, conn, "C 100%")
	d := insertBurger(ctx, t, conn, "D 100%")
	for i, id := range []string{a, b, c, d} {
		shop := public
		if id == d {
			shop = hidden
		}
		linkShopBurger(ctx, t, conn, shop, id)
		insertBurgerStats(ctx, t, conn, id, float64(i+1))
		if _, err := conn.Exec(ctx, "UPDATE burgers SET created_at = $2 WHERE id = $1", id, time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	q := query.NewBurgerQuery(conn)
	for _, tt := range []struct {
		name   string
		filter usecase.BurgerListFilter
		want   []string
	}{
		{"名前順は全対象から並べる", usecase.BurgerListFilter{Sort: "name"}, []string{a, b, c}},
		{"新着順は全対象から並べる", usecase.BurgerListFilter{Sort: "newest"}, []string{c, b, a}},
		{"既定はランキング順を維持する", usecase.BurgerListFilter{}, []string{c, b, a}},
		{"検索記号を文字として扱い非公開の結果を除く", usecase.BurgerListFilter{Keyword: "100%", Sort: "name"}, []string{a, c}},
		{"大文字小文字を区別せず後続ページの名前を検索する", usecase.BurgerListFilter{Keyword: "a 100", Sort: "newest"}, []string{a}},
		{"該当なしは空の結果", usecase.BurgerListFilter{Keyword: "存在しない"}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := []string{}
			for offset := int32(0); ; offset++ {
				rows, more, err := q.ListBurgerRankings(ctx, tt.filter, 1, offset)
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range rows {
					got = append(got, r.ID)
				}
				if !more {
					break
				}
				if offset > 4 {
					t.Fatal("ページングが終了しない")
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
	// 同点でもID順でページ境界が安定する。
	if _, err := conn.Exec(ctx, "UPDATE burger_stats SET weighted_score=4"); err != nil {
		t.Fatal(err)
	}
	rows, _, err := q.ListBurgerRankings(ctx, usecase.BurgerListFilter{}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].ID >= rows[i].ID {
			t.Fatal("同点のID順が不安定")
		}
	}
}

func TestShopRatingSort(t *testing.T) {
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	ids := []string{}
	for i, name := range []string{"C 対象", "B 対象", "A 対象", "非公開 対象"} {
		status := int16(1)
		if i == 3 {
			status = 0
		}
		id := insertShopAt(ctx, t, conn, name, status, time.Now())
		ids = append(ids, id)
		if i != 0 {
			if _, err := conn.Exec(ctx, "INSERT INTO shop_stats(shop_id,review_count,average_rating,calculated_at) VALUES($1,1,4,now())", id); err != nil {
				t.Fatal(err)
			}
		}
	}
	q := query.NewShopQuery(conn)
	got := []string{}
	for offset := int32(0); offset < 3; offset++ {
		rows, more, err := q.ListShops(ctx, domain.ShopVisibilityFor(nil), "対象", usecase.ShopSortRating, 1, offset)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || more != (offset < 2) {
			t.Fatalf("page %d rows %d more %v", offset, len(rows), more)
		}
		got = append(got, rows[0].Shop.ID)
	}
	if !reflect.DeepEqual(got, []string{ids[2], ids[1], ids[0]}) {
		t.Fatalf("order %v", got)
	}
}

func TestReviewRatingSort(t *testing.T) {
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	user := dbtest.InsertUserRow(ctx, t, conn, "INSERT INTO users(email,username,password_digest) VALUES('search@example.com','投稿者','x') RETURNING id")
	shop := insertShopAt(ctx, t, conn, "公開", 1, time.Now())
	burger := insertBurger(ctx, t, conn, "バーガー")
	linkShopBurger(ctx, t, conn, shop, burger)
	ids := []string{}
	for i, rating := range []int{5, 2, 5} {
		id := dbtest.InsertUUIDRow(ctx, t, conn, "INSERT INTO reviews(user_id,burger_id,rating,comment,created_at) VALUES($1,$2,$3,'対象',$4) RETURNING id", user, burger, rating, time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC))
		ids = append(ids, id)
	}
	q := query.NewReviewQuery(conn)
	got := []string{}
	for offset := int32(0); offset < 3; offset++ {
		rows, more, err := q.ListReviews(ctx, usecase.ReviewListFilter{Keyword: "対象", Sort: "rating"}, 1, offset)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || more != (offset < 2) {
			t.Fatalf("page %d rows %d more %v", offset, len(rows), more)
		}
		got = append(got, rows[0].ID)
	}
	if !reflect.DeepEqual(got, []string{ids[2], ids[0], ids[1]}) {
		t.Fatalf("order %v", got)
	}
}
