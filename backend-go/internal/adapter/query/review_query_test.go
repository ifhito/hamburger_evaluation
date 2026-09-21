package query_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// reviewIDs は review の id を順序を保ったまま取り出す。
func reviewIDs(reviews []domain.ReviewDetail) []int64 {
	ids := make([]int64, 0, len(reviews))
	for _, r := range reviews {
		ids = append(ids, r.ID)
	}
	return ids
}

// TestReviewQuery は、review の読み取り（adapter/query の ReviewQuery）を、共有の dbtest の
// スキャフォールドを通じて実際の PostgreSQL に対して検証する（TEST_DATABASE_URL がなければ
// スキップする）。対象は、active な shop に対する EXISTS のフィード絞り込み（重複した link の
// dedup を含む）、soft delete の除外、結合された詳細、filter、ページングである。データは
// SQL の INSERT で用意し、書き込みの adapter/repository には依存しない。
func TestReviewQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := dbtest.InsertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
	active1 := dbtest.InsertRow(ctx, t, conn, insertShop, "Active One", 1, nil, alice)
	active2 := dbtest.InsertRow(ctx, t, conn, insertShop, "Active Two", 1, nil, nil)
	pending := dbtest.InsertRow(ctx, t, conn, insertShop, "Pending Shack", 0, nil, alice)
	rejected := dbtest.InsertRow(ctx, t, conn, insertShop, "Rejected Grill", 2, nil, nil)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := dbtest.InsertRow(ctx, t, conn, insertBurger, "Cheese")   // active な shop の「両方」に link されている
	plain := dbtest.InsertRow(ctx, t, conn, insertBurger, "Plain")     // active1 だけに link されている。stats なし
	hidden := dbtest.InsertRow(ctx, t, conn, insertBurger, "Hidden")   // pending だけに link されている
	outcast := dbtest.InsertRow(ctx, t, conn, insertBurger, "Outcast") // rejected だけに link されている
	mustLink := func(shopID, burgerID int64) {
		t.Helper()
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
			t.Fatalf("link shop %d burger %d: %v", shopID, burgerID, err)
		}
	}
	mustLink(active1, cheese)
	mustLink(active2, cheese)
	mustLink(active1, plain)
	mustLink(pending, hidden)
	mustLink(rejected, outcast)

	// 読み取り系のサブテストは、cheese についてこのリテラルの stats をアサートする。
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 2, 4.0, 3.9, 0.7, now())`, cheese); err != nil {
		t.Fatalf("seed burger stats: %v", err)
	}

	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	t1 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
	rOld := dbtest.InsertRow(ctx, t, conn, insertReview, 5, "Tasty", alice, cheese, nil, t1)
	rTie1 := dbtest.InsertRow(ctx, t, conn, insertReview, 3, nil, carol, cheese, nil, t2)
	rTie2 := dbtest.InsertRow(ctx, t, conn, insertReview, 4, nil, alice, plain, nil, t2) // 同一時刻：id desc で同順位を解消する
	rDiscarded := dbtest.InsertRow(ctx, t, conn, insertReview, 1, "gone", alice, cheese, time.Now(), t2)
	dbtest.InsertRow(ctx, t, conn, insertReview, 2, "pending only", alice, hidden, nil, t2)
	dbtest.InsertRow(ctx, t, conn, insertReview, 2, "rejected only", alice, outcast, nil, t2)

	t.Run("ListReviews は active な shop の burger に絞り込み、重複なしで新しい順に返す", func(t *testing.T) {
		reviews, _, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		// cheese の review は、cheese が active な 2 つの shop に link されて
		// いても、ちょうど 1 回だけ現れなければならない（行を増殖させる JOIN
		// ではなく EXISTS）。pending だけ・rejected だけの burger の review と、
		// discard 済みの review は現れない（SQL レベルでの AC5/AC6）。
		if got, want := reviewIDs(reviews), []int64{rTie2, rTie1, rOld}; !reflect.DeepEqual(got, want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}

		comment := "Tasty"
		wantOld := domain.ReviewDetail{
			Review: domain.Review{ID: rOld, Rating: 5, Comment: &comment, AuthorID: alice, BurgerID: cheese, CreatedAt: t1},
			User:   &domain.UserRef{ID: alice, Username: "alice"},
			Burger: &domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7},
		}
		got := reviews[2]
		if !got.CreatedAt.Equal(wantOld.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, wantOld.CreatedAt)
		}
		got.CreatedAt = wantOld.CreatedAt
		if !reflect.DeepEqual(got, wantOld) {
			t.Errorf("review = %+v, want %+v", got, wantOld)
		}
		// stats のない burger はゼロになる。
		if want := (&domain.ShopReviewBurger{ID: plain, Name: "Plain"}); !reflect.DeepEqual(reviews[0].Burger, want) {
			t.Errorf("stats-less burger = %+v, want %+v", reviews[0].Burger, want)
		}
	})

	t.Run("ListReviews は順序付きフィードを pagination する", func(t *testing.T) {
		page1, more1, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page1), []int64{rTie2, rTie1}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 1 = %v, want %v", got, want)
		}
		page2, more2, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 2)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page2), []int64{rOld}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 2 = %v, want %v", got, want)
		}
		far, moreFar, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 100)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(far) != 0 {
			t.Errorf("far page = %v, want empty", far)
		}
		// has_more: 3 件を 2 件ずつ読むと、1 ページ目だけ続きがある。範囲外は空で false。
		if !more1 || more2 || moreFar {
			t.Errorf("hasMore = (page1 %v, page2 %v, far %v), want (true, false, false)", more1, more2, moreFar)
		}
	})

	t.Run("ListReviews の has_more は、件数ちょうどの limit では false になる（空のページを取りに行かせない）", func(t *testing.T) {
		// フィードは 3 件。limit=3 は件数ちょうど、limit=100 は件数より大きい。
		for _, limit := range []int32{3, 100} {
			all, hasMore, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, limit, 0)
			if err != nil {
				t.Fatalf("ListReviews(limit %d) returned error: %v", limit, err)
			}
			if len(all) != 3 || hasMore {
				t.Errorf("limit %d: len = %d, hasMore = %v, want 3, false", limit, len(all), hasMore)
			}
		}
	})

	t.Run("ListReviews はフィードのルールに加えて Rails ReviewQuery の filter を適用する", func(t *testing.T) {
		intp := func(n int) *int { return &n }
		int64p := func(n int64) *int64 { return &n }
		tests := []struct {
			name   string
			filter usecase.ReviewListFilter
			want   []int64
		}{
			{name: "rating の完全一致 (by_rating)", filter: usecase.ReviewListFilter{Rating: intp(4)}, want: []int64{rTie2}},
			// rating 2 の review は pending だけ・rejected だけの burger にしか
			// 存在しない：active な shop のフィードのルールが引き続き適用される。
			{name: "rating が非表示の review にしか一致しない場合は空になる", filter: usecase.ReviewListFilter{Rating: intp(2)}, want: []int64{}},
			{name: "keyword は大文字小文字を区別しない (keyword_search ILIKE)", filter: usecase.ReviewListFilter{Keyword: "tAsT"}, want: []int64{rOld}},
			// NULL の comment は決して一致しない。Rails の comment ILIKE と
			// 同様である。
			{name: "keyword は NULL の comment を対象にしない", filter: usecase.ReviewListFilter{Keyword: "a"}, want: []int64{rOld}},
			// エスケープしなければ、"%" は NULL でないすべての comment に
			// ILIKE で一致してしまう。
			{name: "keyword の LIKE メタ文字はリテラルとして一致する", filter: usecase.ReviewListFilter{Keyword: "%"}, want: []int64{}},
			{name: "keyword が非表示の review にしか一致しない場合は空になる", filter: usecase.ReviewListFilter{Keyword: "only"}, want: []int64{}},
			{name: "shop_id は shops_burgers の link をたどる", filter: usecase.ReviewListFilter{ShopID: int64p(active2)}, want: []int64{rTie1, rOld}},
			{name: "shop_id で絞り込んでも、その shop の burger の review はすべて残る", filter: usecase.ReviewListFilter{ShopID: int64p(active1)}, want: []int64{rTie2, rTie1, rOld}},
			{name: "存在しない shop_id は空になる", filter: usecase.ReviewListFilter{ShopID: int64p(99999)}, want: []int64{}},
			{name: "filter は AND で組み合わされる", filter: usecase.ReviewListFilter{Rating: intp(5), Keyword: "tast", ShopID: int64p(active2)}, want: []int64{rOld}},
			{name: "AND の組み合わせが一致しない場合は空になる", filter: usecase.ReviewListFilter{Rating: intp(3), Keyword: "tast"}, want: []int64{}},
			// 範囲外の rating は比較結果が false にならなければならず、
			// smallint カラムをオーバーフローさせて SQL エラーに
			// なってはならない。
			{name: "smallint を超える rating は空になり、エラーにならない", filter: usecase.ReviewListFilter{Rating: intp(1 << 40)}, want: []int64{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reviews, _, err := reviewQuery.ListReviews(ctx, tt.filter, 100, 0)
				if err != nil {
					t.Fatalf("ListReviews returned error: %v", err)
				}
				if got := reviewIDs(reviews); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("ids = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("shop_id の filter は、指定された shop 自体が active であることを要求する", func(t *testing.T) {
		// active と pending の「両方」の shop に link された burger：その
		// review は（active な link 経由で）フィードに含まれるが、pending な
		// shop で絞り込むと何も返してはならない。これは Rails の、status を
		// 見ない shop の filter よりも厳格であり、フィードの active な shop の
		// ルールと整合する。
		mixedActive := dbtest.InsertRow(ctx, t, conn, insertShop, "Mixed Active", 1, nil, nil)
		mixed := dbtest.InsertRow(ctx, t, conn, insertBurger, "Mixed")
		mustLink(mixedActive, mixed)
		mustLink(pending, mixed)
		rMixed := dbtest.InsertRow(ctx, t, conn, insertReview, 4, "mixed", alice, mixed, nil, t2)
		t.Cleanup(func() {
			for _, del := range []struct {
				sql string
				id  int64
			}{
				{`DELETE FROM reviews WHERE id = $1`, rMixed},
				{`DELETE FROM shops_burgers WHERE burger_id = $1`, mixed},
				{`DELETE FROM burgers WHERE id = $1`, mixed},
				{`DELETE FROM shops WHERE id = $1`, mixedActive},
			} {
				if _, err := conn.Exec(ctx, del.sql, del.id); err != nil {
					t.Fatalf("cleanup %q: %v", del.sql, err)
				}
			}
		})

		byShop := func(id int64) usecase.ReviewListFilter { return usecase.ReviewListFilter{ShopID: &id} }
		got, _, err := reviewQuery.ListReviews(ctx, byShop(pending), 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("pending shop filter = %v, want empty", reviewIDs(got))
		}
		got, _, err = reviewQuery.ListReviews(ctx, byShop(mixedActive), 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if want := []int64{rMixed}; !reflect.DeepEqual(reviewIDs(got), want) {
			t.Errorf("active shop filter = %v, want %v", reviewIDs(got), want)
		}
	})

	t.Run("GetReview は author、burger、stats を join する", func(t *testing.T) {
		got, err := reviewQuery.GetReview(ctx, rTie1)
		if err != nil {
			t.Fatalf("GetReview returned error: %v", err)
		}
		want := domain.ReviewDetail{
			Review: domain.Review{ID: rTie1, Rating: 3, AuthorID: carol, BurgerID: cheese, CreatedAt: t2},
			User:   &domain.UserRef{ID: carol, Username: "carol"},
			Burger: &domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7},
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
		}
		got.CreatedAt = want.CreatedAt
		if !reflect.DeepEqual(got, want) {
			t.Errorf("detail = %+v, want %+v", got, want)
		}
	})

	t.Run("AC6 discard 済みの review と存在しない review は ErrReviewNotFound になる", func(t *testing.T) {
		for name, id := range map[string]int64{"discarded": rDiscarded, "unknown": 99999} {
			if _, err := reviewQuery.GetReview(ctx, id); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("GetShop は shop 単体を返すか ErrShopNotFound を返す", func(t *testing.T) {
		shop, err := reviewQuery.GetShop(ctx, pending)
		if err != nil {
			t.Fatalf("GetShop returned error: %v", err)
		}
		if shop.ID != pending || shop.Status != domain.ShopStatusPending || shop.CreatorID == nil || *shop.CreatorID != alice {
			t.Errorf("shop = %+v, want pending shop created by alice", shop)
		}
		if _, err := reviewQuery.GetShop(ctx, 99999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("GetShopBurger は shops_burgers の link を要求する", func(t *testing.T) {
		burger, err := reviewQuery.GetShopBurger(ctx, active1, cheese)
		if err != nil {
			t.Fatalf("GetShopBurger returned error: %v", err)
		}
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if !reflect.DeepEqual(burger, want) {
			t.Errorf("burger = %+v, want %+v", burger, want)
		}
		// stats 行がない場合：ゼロ。
		statless, err := reviewQuery.GetShopBurger(ctx, active1, plain)
		if err != nil {
			t.Fatalf("GetShopBurger returned error: %v", err)
		}
		if want := (domain.ShopReviewBurger{ID: plain, Name: "Plain"}); !reflect.DeepEqual(statless, want) {
			t.Errorf("stats-less burger = %+v, want %+v", statless, want)
		}
		// 別の shop に属する既存の burger と未知の burger は、区別できない。
		for name, burgerID := range map[string]int64{"unlinked": hidden, "unknown": 99999} {
			if _, err := reviewQuery.GetShopBurger(ctx, active1, burgerID); !errors.Is(err, domain.ErrBurgerNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrBurgerNotFound)
			}
		}
	})
}

// TestReviewQueryListByUser は、ListReviews の UserID フィルタを実際の
// PostgreSQL に対して検証する：その user の公開 review だけが新しい順に
// pagination されること、rating との AND、discard 済み・存在しない user が
// 空になること、そして「新しいフィルタが公開ルール（discard 済みの review、
// active な shop に紐づかない burger）を迂回しない」ことである。
func TestReviewQueryListByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	bob := dbtest.InsertUserRow(ctx, t, conn, insertUser, "bob@example.com", "bob", false)
	carol := dbtest.InsertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)
	dave := dbtest.InsertUserRow(ctx, t, conn, insertUser, "dave@example.com", "dave", false)
	erin := dbtest.InsertUserRow(ctx, t, conn, insertUser, "erin@example.com", "erin", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
	activeShop := dbtest.InsertRow(ctx, t, conn, insertShop, "Active Shop", 1, nil, nil)
	pendingShop := dbtest.InsertRow(ctx, t, conn, insertShop, "Pending Shop", 0, nil, dave)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := dbtest.InsertRow(ctx, t, conn, insertBurger, "Cheese") // active な shop に link されている
	secret := dbtest.InsertRow(ctx, t, conn, insertBurger, "Secret") // pending な shop だけに link されている
	for _, link := range [][2]int64{{activeShop, cheese}, {pendingShop, secret}} {
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, link[0], link[1]); err != nil {
			t.Fatalf("link shop %d burger %d: %v", link[0], link[1], err)
		}
	}

	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	base := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

	// alice の公開 review 25 件と、bob の公開 review 30 件。時刻は重なっており、
	// 一部は同一時刻（id desc で同順位を解消する）なので、フィルタがなければ
	// 2 人の review は混ざる。
	var aliceIDs, aliceRating5IDs []int64
	for i := 0; i < 25; i++ {
		rating := 1 + i%5
		id := dbtest.InsertRow(ctx, t, conn, insertReview, rating, fmt.Sprintf("alice %d", i), alice, cheese, nil, base.Add(time.Duration(i/2)*time.Minute))
		aliceIDs = append(aliceIDs, id)
		if rating == 5 {
			aliceRating5IDs = append(aliceRating5IDs, id)
		}
	}
	for i := 0; i < 30; i++ {
		dbtest.InsertRow(ctx, t, conn, insertReview, 1+i%5, fmt.Sprintf("bob %d", i), bob, cheese, nil, base.Add(time.Duration(i/3)*time.Minute))
	}
	// created_at は挿入順に単調非減少で id は単調増加なので、新しい順
	// （created_at desc、id desc）は挿入順の逆になる。
	slices.Reverse(aliceIDs)
	slices.Reverse(aliceRating5IDs)

	list := func(t *testing.T, filter usecase.ReviewListFilter, limit, offset int32) []int64 {
		t.Helper()
		reviews, _, err := reviewQuery.ListReviews(ctx, filter, limit, offset)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		return reviewIDs(reviews)
	}
	byUser := func(id string) usecase.ReviewListFilter { return usecase.ReviewListFilter{UserID: &id} }

	t.Run("AC1 UserID はその user の公開 review だけを新しい順に pagination して返す", func(t *testing.T) {
		if got, want := list(t, byUser(alice), 20, 0), aliceIDs[:20]; !reflect.DeepEqual(got, want) {
			t.Errorf("page 1 = %v, want %v", got, want)
		}
		if got, want := list(t, byUser(alice), 20, 20), aliceIDs[20:]; !reflect.DeepEqual(got, want) {
			t.Errorf("page 2 = %v, want %v", got, want)
		}
		if got := list(t, byUser(alice), 20, 40); len(got) != 0 {
			t.Errorf("page 3 = %v, want empty", got)
		}
		// 1 ページに収まる場合も、alice の 25 件だけがちょうど返る（bob の 30 件は
		// 混ざらない）。
		if got := list(t, byUser(alice), 100, 0); !reflect.DeepEqual(got, aliceIDs) {
			t.Errorf("all = %v, want %v", got, aliceIDs)
		}
		if got := list(t, usecase.ReviewListFilter{}, 100, 0); len(got) != 55 {
			t.Errorf("unfiltered feed has %d reviews, want 55 (alice 25 + bob 30)", len(got))
		}
	})

	t.Run("AC2 UserID は rating と AND で組み合わされる", func(t *testing.T) {
		// alice の rating 5 だけが残る。alice の他の rating と、bob の rating 5 は
		// 返らない。
		rating := 5
		filter := byUser(alice)
		filter.Rating = &rating
		if got := list(t, filter, 100, 0); !reflect.DeepEqual(got, aliceRating5IDs) {
			t.Errorf("alice rating 5 = %v, want %v", got, aliceRating5IDs)
		}
	})

	t.Run("AC4 discard 済みの user と存在しない user の id は空になる", func(t *testing.T) {
		carolReview := dbtest.InsertRow(ctx, t, conn, insertReview, 4, "carol was here", carol, cheese, nil, base)
		// discard する前は、carol の review は見える（このテストが空を検証する
		// 意味を持つための前提）。
		if got, want := list(t, byUser(carol), 100, 0), []int64{carolReview}; !reflect.DeepEqual(got, want) {
			t.Fatalf("active carol = %v, want %v", got, want)
		}
		if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, carol); err != nil {
			t.Fatalf("discard carol: %v", err)
		}
		if got := list(t, byUser(carol), 100, 0); !reflect.DeepEqual(got, []int64{}) {
			t.Errorf("discarded carol = %v, want empty", got)
		}
		if got := list(t, byUser(uid.N(999999)), 100, 0); !reflect.DeepEqual(got, []int64{}) {
			t.Errorf("unknown user = %v, want empty", got)
		}
	})

	t.Run("AC5 pending な shop の burger だけの review しかない user は空になり、公開ルールを迂回しない", func(t *testing.T) {
		daveA := dbtest.InsertRow(ctx, t, conn, insertReview, 5, "dave secret 1", dave, secret, nil, base)
		daveB := dbtest.InsertRow(ctx, t, conn, insertReview, 4, "dave secret 2", dave, secret, nil, base.Add(time.Minute))
		// UserID なしのフィードにも現れない（公開ルールの前提）。
		for _, id := range list(t, usecase.ReviewListFilter{}, 100, 0) {
			if id == daveA || id == daveB {
				t.Fatalf("review %d on a pending-only burger leaked into the public feed", id)
			}
		}
		if got := list(t, byUser(dave), 100, 0); !reflect.DeepEqual(got, []int64{}) {
			t.Errorf("dave (pending only) = %v, want empty", got)
		}
	})

	t.Run("AC5 UserID を指定しても discard 済みの review と非公開の burger の review は除外される", func(t *testing.T) {
		kept := dbtest.InsertRow(ctx, t, conn, insertReview, 4, "erin kept", erin, cheese, nil, base)
		dbtest.InsertRow(ctx, t, conn, insertReview, 1, "erin discarded", erin, cheese, time.Now(), base.Add(time.Minute))
		dbtest.InsertRow(ctx, t, conn, insertReview, 5, "erin on pending", erin, secret, nil, base.Add(2*time.Minute))
		if got, want := list(t, byUser(erin), 100, 0), []int64{kept}; !reflect.DeepEqual(got, want) {
			t.Errorf("erin = %v, want only the kept review on the active shop %v", got, want)
		}
	})
}

// TestReviewQueryPhotoKey は、S10 の photo_key を、結合された読み取りクエリが返すことを検証する。
// データは SQL の INSERT で用意する。
func TestReviewQueryPhotoKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)

	alice := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`,
		"alice@example.com", "alice")
	shop := dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO shops (name, status) VALUES ($1, 1) RETURNING id`, "Active One")
	burger := dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shop, burger); err != nil {
		t.Fatalf("link shop and burger: %v", err)
	}
	reviewID := dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO reviews (rating, comment, user_id, burger_id, photo_key) VALUES (4, 'Tasty', $1, $2, 'reviews/abc.jpg') RETURNING id`,
		alice, burger)

	t.Run("読み取りクエリは photo_key を返す", func(t *testing.T) {
		detail, err := reviewQuery.GetReview(ctx, reviewID)
		if err != nil {
			t.Fatalf("GetReview returned error: %v", err)
		}
		if detail.PhotoKey == nil || *detail.PhotoKey != "reviews/abc.jpg" {
			t.Errorf("detail PhotoKey = %v, want reviews/abc.jpg", detail.PhotoKey)
		}
		list, _, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 10, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(list) != 1 || list[0].PhotoKey == nil || *list[0].PhotoKey != "reviews/abc.jpg" {
			t.Errorf("list = %+v, want one review with PhotoKey reviews/abc.jpg", list)
		}
	})
}

// TestReviewQueryDiscardedUser は、退会（discard）した user の kept な review を、読み取り側が
// フィード・review の詳細・shop の review から除外することを検証する。データは SQL の INSERT で
// 用意し（退会は discarded_at の UPDATE）、repository を使わない。
func TestReviewQueryDiscardedUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)
	shopQuery := query.NewShopQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, $3, $4) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", "digest-alice", false)
	victim := dbtest.InsertUserRow(ctx, t, conn, insertUser, "victim@example.com", "victim", "digest-victim", false)
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, victim); err != nil {
		t.Fatalf("discard victim: %v", err)
	}

	// active な shop 1 つが 2 つの burger を提供している："shared" は victim と alice の両方が
	// review し、"solo" は victim だけが review した。victim の review は kept のまま（非表示化は
	// 純粋に読み取り側で行われる）。shared の burger_stats は、victim を除いた alice の review だけの
	// 値（count 1）が保存されている、という状態にする。
	shop := dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Active One", 1, nil, nil)
	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	shared := dbtest.InsertRow(ctx, t, conn, insertBurger, "Shared")
	solo := dbtest.InsertRow(ctx, t, conn, insertBurger, "Solo")
	for _, burgerID := range []int64{shared, solo} {
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shop, burgerID); err != nil {
			t.Fatalf("link shop %d burger %d: %v", shop, burgerID, err)
		}
	}
	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id) VALUES ($1, $2, $3, $4) RETURNING id`
	victimShared := dbtest.InsertRow(ctx, t, conn, insertReview, 2, "meh", victim, shared)
	aliceShared := dbtest.InsertRow(ctx, t, conn, insertReview, 4, "good", alice, shared)
	victimSolo := dbtest.InsertRow(ctx, t, conn, insertReview, 5, "only mine", victim, solo)
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 1, 4.0, 3.9, 0.7, now())`, shared); err != nil {
		t.Fatalf("seed burger stats: %v", err)
	}

	t.Run("読み取り経路は discard 済みの user の kept な review を隠す", func(t *testing.T) {
		// フィード：alice の review だけが残り、表示される stats は再計算された
		// burger_stats の行（count 1）と一致する。
		feed, _, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(feed), []int64{aliceShared}; !reflect.DeepEqual(got, want) {
			t.Fatalf("feed ids = %v, want %v (victim's reviews hidden)", got, want)
		}
		var storedCount int64
		if err := conn.QueryRow(ctx, `SELECT review_count FROM burger_stats WHERE burger_id = $1`, shared).Scan(&storedCount); err != nil {
			t.Fatalf("select burger_stats: %v", err)
		}
		if feed[0].Burger.ReviewCount != storedCount || feed[0].Burger.ReviewCount != int64(len(feed)) {
			t.Errorf("displayed review count = %d, want burger_stats %d = %d displayed reviews",
				feed[0].Burger.ReviewCount, storedCount, len(feed))
		}
		// 詳細：discard 済みの author の review は、存在しない review と
		// 区別がつかない。alice の review には引き続き到達できる。
		if _, err := reviewQuery.GetReview(ctx, victimShared); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim shared) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := reviewQuery.GetReview(ctx, victimSolo); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim solo) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := reviewQuery.GetReview(ctx, aliceShared); err != nil {
			t.Errorf("GetReview(alice) returned error: %v", err)
		}
		// shop の review：alice の review だけが一覧に載る。
		shopReviews, err := shopQuery.ListShopReviews(ctx, shop)
		if err != nil {
			t.Fatalf("ListShopReviews returned error: %v", err)
		}
		ids := make([]int64, 0, len(shopReviews))
		for _, r := range shopReviews {
			ids = append(ids, r.ID)
		}
		if want := []int64{aliceShared}; !reflect.DeepEqual(ids, want) {
			t.Errorf("shop review ids = %v, want %v (victim's review hidden)", ids, want)
		}
	})
}
