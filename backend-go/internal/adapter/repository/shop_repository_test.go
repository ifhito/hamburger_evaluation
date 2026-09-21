package repository_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// このファイルは adapter の DB 統合テストである。読み取りの adapter/query と
// 書き込みの adapter/repository の両方を実 PostgreSQL で検証する（fixture を
// 共有するため、外部テスト package の repository_test に置いている）。

// insertRow は sql（id を RETURN する必要がある）で insert し、新しい id を
// 返す。
func insertRow(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := conn.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

// insertUserRow は insertRow と同様だが、users の id(UUID の正規形の文字列)を返す。
func insertUserRow(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

// shopIDs は、順序に依存しない比較のために shop の id を取り出す。
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

// shopNames は、keyword と順序のアサーションのために、順序を保ったまま
// name を取り出す。
func shopNames(shops []domain.Shop) []string {
	names := make([]string, 0, len(shops))
	for _, s := range shops {
		names = append(names, s.Name)
	}
	return names
}

// TestShopRepository は、shop の読み取り（adapter/query の ShopQuery）を、
// 共有の dbtest のスキャフォールドを通じて実際の PostgreSQL に対して検証する
// （TEST_DATABASE_URL がなければスキップする）。issue #12 の AC1–AC3 と AC5 を
// SQL レベルで扱うほか、pagination、順序、両方の詳細クエリを扱う。
func TestShopRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	shopQuery := query.NewShopQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := insertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
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

	t.Run("AC3 admin はすべての status の shop を見られる", func(t *testing.T) {
		want := sortedIDs(append([]int64{alicePending, golfRejected}, activeIDs...)...)
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
		if shops[0].ID != orderA1 || shops[1].ID != orderA2 {
			t.Errorf("same-name ids = %d,%d, want %d,%d (id asc)", shops[0].ID, shops[1].ID, orderA1, orderA2)
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
		if _, err := shopQuery.GetShopWithCreator(ctx, 99999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("ListShopReviews は user、burger、stats を join して順序どおりに返す", func(t *testing.T) {
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
		r3 := insertRow(ctx, t, conn, insertReview, 4, nil, alice, plain, nil, t2) // r2 と同時刻：id desc で同順位を解消する
		insertRow(ctx, t, conn, insertReview, 1, "discarded", alice, cheese, time.Now(), t2)
		insertRow(ctx, t, conn, insertReview, 2, "other shop", alice, other, nil, t2)
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

// TestShopModerationRepository は、S5 の投稿と moderation の永続化を、実際の
// PostgreSQL に対して検証する。pending の shop の作成、admin の moderation
// 一覧（順序、filter、creator の join）、moderation の update、そして
// approve/reject が visibility にもたらす結果である。
func TestShopModerationRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewShopRepository(conn)
	shopQuery := query.NewShopQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。created_at の値を
	// 明示することで moderation 一覧の順序を固定する。old1/old2 は同一の
	// instant を共有するので、id desc が同順位を解消しなければならない。
	tOld := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	tNew := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	old1 := insertRow(ctx, t, conn, insertShop, "Old One", 1, nil, alice, tOld)
	old2 := insertRow(ctx, t, conn, insertShop, "Old Two", 2, "needs fixes", nil, tOld)
	newest := insertRow(ctx, t, conn, insertShop, "Newest", 0, nil, alice, tNew)

	anon := domain.ShopVisibilityFor(nil)
	aliceVis := domain.ShopVisibilityFor(&domain.User{ID: alice})

	t.Run("CreateShop は creator 付きの pending な shop を永続化する", func(t *testing.T) {
		submission, err := domain.NewShopSubmission("Fresh Shack", alice)
		if err != nil {
			t.Fatalf("NewShopSubmission returned error: %v", err)
		}
		created, err := repo.CreateShop(ctx, submission)
		if err != nil {
			t.Fatalf("CreateShop returned error: %v", err)
		}
		if created.ID == 0 || created.Status != domain.ShopStatusPending || created.ModerationNote != nil {
			t.Errorf("created = %+v, want generated id, pending, nil note", created)
		}
		detail, err := shopQuery.GetShopWithCreator(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if !reflect.DeepEqual(detail.Creator, &domain.UserRef{ID: alice, Username: "alice"}) {
			t.Errorf("creator = %+v, want alice", detail.Creator)
		}

		// S4 の visibility のルールは、作成直後の shop でも成り立つ：
		// creator には一覧に見え、匿名の viewer には見えない。
		anonShops, _, err := shopQuery.ListShops(ctx, anon, "Fresh Shack", 100, 0)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		if len(anonShops) != 0 {
			t.Errorf("anonymous list = %v, want empty", anonShops)
		}
		ownShops, _, err := shopQuery.ListShops(ctx, aliceVis, "Fresh Shack", 100, 0)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		if len(ownShops) != 1 || ownShops[0].ID != created.ID {
			t.Errorf("creator list = %v, want the created shop", ownShops)
		}

		// 下の順序のアサーションが厳密なままになるよう、これを再度削除する。
		if _, err := conn.Exec(ctx, `DELETE FROM shops WHERE id = $1`, created.ID); err != nil {
			t.Fatalf("delete created shop: %v", err)
		}
	})

	t.Run("ListShopsForModeration は created_at 降順、次に id 降順に並べる", func(t *testing.T) {
		shops, err := shopQuery.ListShopsForModeration(ctx, nil)
		if err != nil {
			t.Fatalf("ListShopsForModeration returned error: %v", err)
		}
		ids := make([]int64, 0, len(shops))
		for _, s := range shops {
			ids = append(ids, s.ID)
		}
		if want := []int64{newest, old2, old1}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
		if !reflect.DeepEqual(shops[0].Creator, &domain.UserRef{ID: alice, Username: "alice"}) {
			t.Errorf("creator = %+v, want alice", shops[0].Creator)
		}
		if shops[1].Creator != nil {
			t.Errorf("creatorless shop creator = %+v, want nil", shops[1].Creator)
		}
		if shops[1].ModerationNote == nil || *shops[1].ModerationNote != "needs fixes" {
			t.Errorf("note = %v, want needs fixes", shops[1].ModerationNote)
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

	t.Run("UpdateShopStatus は approve と reject を永続化し、visibility に反映する", func(t *testing.T) {
		detail, err := shopQuery.GetShopWithCreator(ctx, newest)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}

		next := detail.Shop.Approve()
		approved, err := repo.UpdateShopStatus(ctx, newest, next.Status, next.ModerationNote)
		if err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		if approved.Status != domain.ShopStatusActive || approved.ModerationNote != nil {
			t.Errorf("approved = %+v, want active with nil note", approved)
		}
		anonShops, _, err := shopQuery.ListShops(ctx, anon, "Newest", 100, 0)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		if len(anonShops) != 1 || anonShops[0].ID != newest {
			t.Errorf("anonymous list after approve = %v, want the shop", anonShops)
		}

		note := "spam"
		next = approved.Reject(&note)
		rejected, err := repo.UpdateShopStatus(ctx, newest, next.Status, next.ModerationNote)
		if err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		if rejected.Status != domain.ShopStatusRejected || rejected.ModerationNote == nil || *rejected.ModerationNote != note {
			t.Errorf("rejected = %+v, want rejected with the note", rejected)
		}
		stored, err := shopQuery.GetShopWithCreator(ctx, newest)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if stored.Status != domain.ShopStatusRejected || stored.ModerationNote == nil || *stored.ModerationNote != note {
			t.Errorf("stored = %+v, want the persisted rejection", stored.Shop)
		}
		anonShops, _, err = shopQuery.ListShops(ctx, anon, "Newest", 100, 0)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		if len(anonShops) != 0 {
			t.Errorf("anonymous list after reject = %v, want empty", anonShops)
		}
	})

	t.Run("カラム単位の書き込みは並行する更新を巻き戻さない", func(t *testing.T) {
		shop := insertRow(ctx, t, conn, insertShop, "Race Shack", 0, nil, alice, tNew)

		// lost-update の回帰、方向 1：古い rename 側は、並行する approve の
		// 前にスナップショットを読んだ。旧来の行全体の書き込みは status を
		// pending に戻して approve を巻き戻してしまうが、カラム単位の rename は
		// status と note に手を付けてはならない。
		stale, err := shopQuery.GetShopWithCreator(ctx, shop)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if stale.Status != domain.ShopStatusPending {
			t.Fatalf("snapshot status = %q, want pending", stale.Status)
		}
		next := stale.Shop.Approve() // 並行する admin が approve する
		if _, err := repo.UpdateShopStatus(ctx, shop, next.Status, next.ModerationNote); err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		renamed, err := repo.UpdateShopName(ctx, shop, "Race Shack Renamed") // 古い rename 側が書き込む
		if err != nil {
			t.Fatalf("UpdateShopName returned error: %v", err)
		}
		if renamed.Name != "Race Shack Renamed" || renamed.Status != domain.ShopStatusActive || renamed.ModerationNote != nil {
			t.Errorf("after stale rename = %+v, want new name AND active with nil note", renamed)
		}

		// 方向 2：並行する rename の前に取得したスナップショットからの status の
		// 書き込みは、新しい name をそのまま保たなければならない。
		note := "spam"
		next = stale.Shop.Reject(&note)
		rejected, err := repo.UpdateShopStatus(ctx, shop, next.Status, next.ModerationNote)
		if err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		if rejected.Name != "Race Shack Renamed" || rejected.Status != domain.ShopStatusRejected || rejected.ModerationNote == nil || *rejected.ModerationNote != note {
			t.Errorf("after stale status write = %+v, want kept name AND rejected with the note", rejected)
		}

		stored, err := shopQuery.GetShopWithCreator(ctx, shop)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if stored.Name != "Race Shack Renamed" || stored.Status != domain.ShopStatusRejected {
			t.Errorf("stored = %+v, want renamed and rejected", stored.Shop)
		}

		// 兄弟サブテストの順序のアサーションが、実行順序にかかわらず厳密な
		// ままになるよう、これを再度削除する。
		if _, err := conn.Exec(ctx, `DELETE FROM shops WHERE id = $1`, shop); err != nil {
			t.Fatalf("delete race shop: %v", err)
		}
	})

	t.Run("UpdateShopName に存在しない id を渡すと ErrShopNotFound になる", func(t *testing.T) {
		_, err := repo.UpdateShopName(ctx, 99999, "x")
		if !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("UpdateShopStatus に存在しない id を渡すと ErrShopNotFound になる", func(t *testing.T) {
		_, err := repo.UpdateShopStatus(ctx, 99999, domain.ShopStatusActive, nil)
		if !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})
}
