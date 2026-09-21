package query_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// このファイルは adapter/query の DB 統合テストである。読み取りの SQL（絞り込み・順序・
// ページング・可視性・join）だけを、実 PostgreSQL で検証する。データは SQL の INSERT で
// 用意し、書き込みの adapter/repository には依存しない（書き込みは repository のテストが担う）。

// shopIDs は、順序に依存しない比較のために shop の id を取り出す。
func shopIDs(shops []domain.Shop) []string {
	ids := make([]string, 0, len(shops))
	for _, s := range shops {
		ids = append(ids, s.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedIDs(ids ...string) []string {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// shopNames は、keyword と順序のアサーションのために、順序を保ったまま
// name を取り出す。
func shopNames(shops []domain.Shop) []string {
	names := make([]string, 0, len(shops))
	for _, s := range shops {
		names = append(names, s.Name)
	}
	return names
}

// TestShopQuery は、shop の読み取り（adapter/query の ShopQuery）を、
// 共有の dbtest のスキャフォールドを通じて実際の PostgreSQL に対して検証する
// （TEST_DATABASE_URL がなければスキップする）。issue #12 の AC1–AC3 と AC5 を
// SQL レベルで扱うほか、pagination、順序、両方の詳細クエリを扱う。
func TestShopQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	shopQuery := query.NewShopQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := dbtest.InsertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
	deltaDiner := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Delta Diner", 1, nil, carol)
	alicePending := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Alpha Pending", 0, nil, alice)
	golfRejected := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Golf Grill", 2, "needs fixes", nil)
	pctBeef := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "100% Beef", 1, nil, nil)
	xBeef := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "100x Beef", 1, nil, nil)
	underScore := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Under_score", 1, nil, nil)
	underX := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "UnderXscore", 1, nil, nil)
	backslash := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, `Back\slash Cafe`, 1, nil, nil)
	orderA1 := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Order Cafe A", 1, nil, nil)
	orderA2 := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Order Cafe A", 1, nil, nil)
	orderB := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Order Cafe B", 1, nil, nil)

	activeIDs := sortedIDs(deltaDiner, pctBeef, xBeef, underScore, underX, backslash, orderA1, orderA2, orderB)

	anon := domain.ShopVisibilityFor(nil)
	aliceVis := domain.ShopVisibilityFor(&domain.User{ID: alice})
	adminVis := domain.ShopVisibilityFor(&domain.User{ID: uid.N(999), Admin: true})

	list := func(t *testing.T, vis domain.ShopVisibility, keyword string, limit, offset int32) []domain.Shop {
		t.Helper()
		shops, _, err := shopQuery.ListShops(ctx, vis, keyword, limit, offset)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		return shops
	}

	t.Run("AC1 匿名の viewer には active な shop だけが一覧に出る", func(t *testing.T) {
		if got := shopIDs(list(t, anon, "", 100, 0)); !reflect.DeepEqual(got, activeIDs) {
			t.Errorf("ids = %v, want %v", got, activeIDs)
		}
	})

	t.Run("AC2 creator には自分の pending な shop が status 付きで追加で見える", func(t *testing.T) {
		shops := list(t, aliceVis, "", 100, 0)
		want := sortedIDs(append([]string{alicePending}, activeIDs...)...)
		if got := shopIDs(shops); !reflect.DeepEqual(got, want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
		for _, s := range shops {
			if s.ID == alicePending && s.Status != domain.ShopStatusPending {
				t.Errorf("pending shop status = %q, want %q", s.Status, domain.ShopStatusPending)
			}
		}
	})

	t.Run("AC3 admin はすべての status の shop を見られる", func(t *testing.T) {
		want := sortedIDs(append([]string{alicePending, golfRejected}, activeIDs...)...)
		if got := shopIDs(list(t, adminVis, "", 100, 0)); !reflect.DeepEqual(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})

	t.Run("順序は name 昇順、次に id 昇順になる", func(t *testing.T) {
		shops := list(t, anon, "Order Cafe", 100, 0)
		wantNames := []string{"Order Cafe A", "Order Cafe A", "Order Cafe B"}
		if got := shopNames(shops); !reflect.DeepEqual(got, wantNames) {
			t.Fatalf("names = %v, want %v", got, wantNames)
		}
		// id は UUID なので、同名の 2 件の並びは、id の昇順（文字列としての昇順）になる。
		wantSame := sortedIDs(orderA1, orderA2)
		if shops[0].ID != wantSame[0] || shops[1].ID != wantSame[1] {
			t.Errorf("same-name ids = %s,%s, want %s,%s (id asc)", shops[0].ID, shops[1].ID, wantSame[0], wantSame[1])
		}
	})

	t.Run("pagination は順序付き一覧を切り出し、範囲外では空になる", func(t *testing.T) {
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

	t.Run("has_more は limit+1 件の取得で判定され、ちょうど最後のページでは false になる", func(t *testing.T) {
		// "Order Cafe" は 3 件（A、A、B）。
		tests := []struct {
			name          string
			limit, offset int32
			wantLen       int
			wantMore      bool
		}{
			{name: "途中のページ（3 件を 2 件ずつの 1 ページ目）は true", limit: 2, offset: 0, wantLen: 2, wantMore: true},
			{name: "最後のページ（2 ページ目の 1 件）は false", limit: 2, offset: 2, wantLen: 1, wantMore: false},
			{name: "件数ちょうどの limit（3 件を 3 件）は false（空のページを取りに行かせない）", limit: 3, offset: 0, wantLen: 3, wantMore: false},
			{name: "limit が件数より大きければ false", limit: 100, offset: 0, wantLen: 3, wantMore: false},
			{name: "範囲外の offset は空で false", limit: 2, offset: 100, wantLen: 0, wantMore: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				shops, hasMore, err := shopQuery.ListShops(ctx, anon, "Order Cafe", tt.limit, tt.offset)
				if err != nil {
					t.Fatalf("ListShops returned error: %v", err)
				}
				if len(shops) != tt.wantLen || hasMore != tt.wantMore {
					t.Errorf("len = %d, hasMore = %v, want %d, %v", len(shops), hasMore, tt.wantLen, tt.wantMore)
				}
			})
		}
	})

	t.Run("AC5 keyword は大文字小文字を区別しない部分一致になる", func(t *testing.T) {
		want := sortedIDs(pctBeef, xBeef)
		if got := shopIDs(list(t, anon, "bEEf", 100, 0)); !reflect.DeepEqual(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})

	t.Run("AC5 keyword 中のパーセントはリテラルとして一致する", func(t *testing.T) {
		if got := shopNames(list(t, anon, "100%", 100, 0)); !reflect.DeepEqual(got, []string{"100% Beef"}) {
			t.Errorf("names = %v, want [100%% Beef]", got)
		}
	})

	t.Run("AC5 keyword 中のアンダースコアはリテラルとして一致する", func(t *testing.T) {
		if got := shopNames(list(t, anon, "Under_", 100, 0)); !reflect.DeepEqual(got, []string{"Under_score"}) {
			t.Errorf("names = %v, want [Under_score]", got)
		}
	})

	t.Run("AC5 keyword 中のバックスラッシュはリテラルとして一致する", func(t *testing.T) {
		if got := shopNames(list(t, anon, `\`, 100, 0)); !reflect.DeepEqual(got, []string{`Back\slash Cafe`}) {
			t.Errorf(`names = %v, want [Back\slash Cafe]`, got)
		}
	})

	t.Run("AC5 SQL injection を狙った keyword はエラーにならず、データも漏れない", func(t *testing.T) {
		if got := list(t, anon, `'; DROP TABLE shops;--`, 100, 0); len(got) != 0 {
			t.Errorf("shops = %v, want empty", got)
		}
		// テーブルは依然として無傷でなければならない。
		if got := shopIDs(list(t, anon, "", 100, 0)); !reflect.DeepEqual(got, activeIDs) {
			t.Errorf("ids after injection attempt = %v, want %v", got, activeIDs)
		}
	})

	t.Run("GetShopWithCreator は creator と status を返す", func(t *testing.T) {
		detail, err := shopQuery.GetShopWithCreator(ctx, deltaDiner)
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

	t.Run("GetShopWithCreator は creator が null で moderation note がある shop をマッピングする", func(t *testing.T) {
		detail, err := shopQuery.GetShopWithCreator(ctx, golfRejected)
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

	t.Run("AC6 存在しない shop id は ErrShopNotFound になる", func(t *testing.T) {
		if _, err := shopQuery.GetShopWithCreator(ctx, uid.N(99999)); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("ListShopReviews は user、burger、stats を join して順序どおりに返す", func(t *testing.T) {
		insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
		cheese := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Cheese")
		plain := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Plain")
		other := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Other")
		mustLink := func(shopID, burgerID string) {
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
				t.Fatalf("link shop %s burger %s: %v", shopID, burgerID, err)
			}
		}
		mustLink(deltaDiner, cheese)
		mustLink(deltaDiner, plain)
		mustLink(pctBeef, other)

		insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
		t1 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
		t2 := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
		r1 := dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 5, "Tasty", alice, cheese, nil, t1)
		r2 := dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 3, nil, carol, cheese, nil, t2)
		r3 := dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 4, nil, alice, plain, nil, t2) // r2 と同時刻：id desc で同順位を解消する
		dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 1, "discarded", alice, cheese, time.Now(), t2)
		dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 2, "other shop", alice, other, nil, t2)
		if _, err := conn.Exec(ctx,
			`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
			 VALUES ($1, 2, 4.0, 3.9, 0.7, now())`, cheese); err != nil {
			t.Fatalf("insert burger stats: %v", err)
		}

		reviews, err := shopQuery.ListShopReviews(ctx, deltaDiner)
		if err != nil {
			t.Fatalf("ListShopReviews returned error: %v", err)
		}
		comment := "Tasty"
		want := []domain.ShopReview{
			{
				ID: r3, Rating: 4, CreatedAt: t2,
				User:   &domain.UserRef{ID: alice, Username: "alice"},
				Burger: &domain.ShopReviewBurger{ID: plain, Name: "Plain"}, // stats 行なし：ゼロ
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
		// r3 と r2 は created_at が同じなので、id の降順（UUID は文字列としての降順）で並ぶ。
		if want[0].ID < want[1].ID {
			want[0], want[1] = want[1], want[0]
		}
		if len(reviews) != len(want) {
			t.Fatalf("got %d reviews (%+v), want %d", len(reviews), reviews, len(want))
		}
		for i := range want {
			got := reviews[i]
			// タイムスタンプは instant で比較し（driver はセッションの
			// タイムゾーンで返す）、そのうえで下の DeepEqual のために揃える。
			if !got.CreatedAt.Equal(want[i].CreatedAt) {
				t.Errorf("review[%d].CreatedAt = %v, want %v", i, got.CreatedAt, want[i].CreatedAt)
			}
			got.CreatedAt = want[i].CreatedAt
			if !reflect.DeepEqual(got, want[i]) {
				t.Errorf("review[%d] = %+v, want %+v", i, got, want[i])
			}
		}

		if got, err := shopQuery.ListShopReviews(ctx, golfRejected); err != nil || len(got) != 0 {
			t.Errorf("reviews of shop without burgers = %v, %v; want empty, nil", got, err)
		}
	})
}

// TestShopModerationQuery は、admin の moderation 一覧（ShopQuery.ListShopsForModeration）の
// 順序・filter・creator の join を、実際の PostgreSQL に対して検証する。データは SQL の INSERT で
// 用意し、repository を使わない。
func TestShopModerationQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	shopQuery := query.NewShopQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。created_at の値を
	// 明示することで moderation 一覧の順序を固定する。old1/old2 は同一の
	// instant を共有するので、id desc が同順位を解消しなければならない。
	tOld := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	tNew := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	old1 := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Old One", 1, nil, alice, tOld)
	old2 := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Old Two", 2, "needs fixes", nil, tOld)
	newest := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Newest", 0, nil, alice, tNew)

	t.Run("ListShopsForModeration は created_at 降順、次に id 降順に並べる", func(t *testing.T) {
		shops, err := shopQuery.ListShopsForModeration(ctx, nil)
		if err != nil {
			t.Fatalf("ListShopsForModeration returned error: %v", err)
		}
		ids := make([]string, 0, len(shops))
		for _, s := range shops {
			ids = append(ids, s.ID)
		}
		// old1 と old2 は created_at が同じなので、id の降順（UUID は文字列としての降順）で並ぶ。
		sameOld := sortedIDs(old1, old2)
		if want := []string{newest, sameOld[1], sameOld[0]}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
		// old1 と old2 の並びは id で決まるので、位置ではなく id で取り出す。
		byID := map[string]domain.ShopDetail{}
		for _, s := range shops {
			byID[s.ID] = s
		}
		if !reflect.DeepEqual(byID[newest].Creator, &domain.UserRef{ID: alice, Username: "alice"}) {
			t.Errorf("creator = %+v, want alice", byID[newest].Creator)
		}
		if byID[old2].Creator != nil {
			t.Errorf("creatorless shop creator = %+v, want nil", byID[old2].Creator)
		}
		if note := byID[old2].ModerationNote; note == nil || *note != "needs fixes" {
			t.Errorf("note = %v, want needs fixes", note)
		}
	})

	t.Run("ListShopsForModeration は status で絞り込む", func(t *testing.T) {
		status := domain.ShopStatusRejected
		shops, err := shopQuery.ListShopsForModeration(ctx, &status)
		if err != nil {
			t.Fatalf("ListShopsForModeration returned error: %v", err)
		}
		if len(shops) != 1 || shops[0].ID != old2 {
			t.Errorf("shops = %+v, want only the rejected one", shops)
		}
	})
}
