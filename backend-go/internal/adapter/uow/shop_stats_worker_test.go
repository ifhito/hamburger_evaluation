package uow_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/statsworkertest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// このファイルは、ショップの集計(件数・平均・写真)の再計算を、実際の PostgreSQL に対して、本物の usecase・
// query・repository とつないで確かめる。バーガーの統計のワーカーが、統計を計算し直すときに、そのバーガーが
// 紐づくショップの依頼を同じトランザクションで積み、ショップの集計のワーカーが、依頼をショップごとにまとめて
// 計算し直すこと、再計算の最中の書き込みでも、最新に収束すること、失敗が再試行されることを確かめる。

// settleShops は、溜まったショップの集計の依頼を、ワーカーで処理する。
func (w *world) settleShops(t *testing.T) {
	t.Helper()
	statsworkertest.SettleShops(w.ctx, t, statsworkertest.NewShopWorker(w.conn, infra.SystemClock{}))
}

// settleAll は、バーガーの統計の依頼を処理し(ショップの依頼が積まれる)、続けて、ショップの集計の依頼を処理する。
func (w *world) settleAll(t *testing.T) {
	t.Helper()
	w.settle(t)
	w.settleShops(t)
}

// shopStat は、ショップの保存された集計を読む(行がなければ、テストを失敗させる)。
func (w *world) shopStat(t *testing.T, shopID string) dbtest.StoredShopStats {
	t.Helper()
	stat, ok := dbtest.FetchShopStats(w.ctx, t, w.conn, shopID)
	if !ok {
		t.Fatalf("ショップ %s の集計の行がない", shopID)
	}
	return stat
}

func (w *world) shopRequests(t *testing.T, shopID string) int {
	t.Helper()
	if _, ok := dbtest.FetchShopRecalcRequest(w.ctx, t, w.conn, shopID); ok {
		return 1
	}
	return 0
}

// hookedShopUnit は、UnitOfWork のトランザクションの中で、ショップの集計の元データを読んだ直後に hook を呼ぶ
// UnitOfWork である。元データを読んだあと、集計を保存する前という、再計算の途中の場面を、テストが作り出すために
// 使う。hook が error を返すと、その元データの読み取りは、その error で失敗する。
type hookedShopUnit struct {
	usecase.UnitOfWork
	hook func(ctx context.Context, shopID string) error
}

func (h hookedShopUnit) Do(ctx context.Context, fn func(ctx context.Context, tx usecase.Tx) error) error {
	return h.UnitOfWork.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
		tx.ShopStatsReads = hookedShopStats{ShopStatsQuery: tx.ShopStatsReads, hook: h.hook}
		return fn(ctx, tx)
	})
}

type hookedShopStats struct {
	usecase.ShopStatsQuery
	hook func(ctx context.Context, shopID string) error
}

func (s hookedShopStats) ListShopReviewFacts(ctx context.Context, shopID string) ([]domain.ShopReviewFact, error) {
	facts, err := s.ShopStatsQuery.ListShopReviewFacts(ctx, shopID)
	if err != nil {
		return nil, err
	}
	if err := s.hook(ctx, shopID); err != nil {
		return nil, err
	}
	return facts, nil
}

func (w *world) hookedShopWorker(clock usecase.Clock, maxAttempts int, hook func(ctx context.Context, shopID string) error) *usecase.ShopStatsWorker {
	return usecase.NewShopStatsWorker(query.NewShopStatsQuery(w.conn), hookedShopUnit{UnitOfWork: w.unit, hook: hook},
		usecase.NewShopStatsRecalculator(clock), clock, usecase.StatsWorkerConfig{Batch: 100, MaxAttempts: maxAttempts})
}

// TestShopStatsWorkerChain は、レビューの書き込みから、ショップの集計が反映されるまでの流れ(書き込みが、バーガーの
// 依頼と同じトランザクションで、ショップの依頼も直接登録する → ショップの集計のワーカーが集計を保存する)を確かめる。
// バーガーの統計のワーカーは、ショップの集計には一切触れない(依頼のレビュー指摘への対応。ShopStatsRecalculator の
// コメントに理由がある: ワーカーからワーカーへ連鎖させると、ショップ側の失敗が、健全なバーガーの統計の再計算まで
// 失敗として記録してしまう)。
func TestShopStatsWorkerChain(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn

	t.Run("投稿した時点で、バーガーの依頼と同じトランザクションで、ショップの依頼も積まれる。ショップのワーカーが集計を保存して依頼を消す。バーガーのワーカーは、ショップの依頼に触れない", func(t *testing.T) {
		alice := w.user(t, "chain-alice")
		burger := w.burger(t, "Chain Burger")
		w.review(t, alice, burger, 4, "good")

		if _, ok := dbtest.FetchShopStats(ctx, t, conn, w.shop); ok {
			t.Fatal("投稿しただけで、ショップの集計ができている(集計は、あとからワーカーが計算する)")
		}
		if n := w.shopRequests(t, w.shop); n != 1 {
			t.Fatalf("投稿した時点でのショップの依頼 = %d 件, want 1 件(書き込みが、バーガーの依頼と同じトランザクションで登録する)", n)
		}

		if n, err := w.worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("バーガーのワーカーの RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if n := w.shopRequests(t, w.shop); n != 1 {
			t.Fatalf("バーガーのワーカーのあとのショップの依頼 = %d 件, want 1 件のまま(バーガーのワーカーは、ショップの依頼に触れない)", n)
		}
		if _, ok := dbtest.FetchShopStats(ctx, t, conn, w.shop); ok {
			t.Fatal("ショップのワーカーが動く前に、集計ができている")
		}

		shopWorker := statsworkertest.NewShopWorker(conn, infra.SystemClock{})
		if n, err := shopWorker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("ショップのワーカーの RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		got := w.shopStat(t, w.shop)
		if got.ReviewCount != 1 || got.AverageRating == nil || *got.AverageRating != 4 || got.PhotoKey != nil {
			t.Errorf("集計 = %+v, want 件数 1・平均 4・写真なし", got)
		}
		if n := w.shopRequests(t, w.shop); n != 0 {
			t.Errorf("再計算のあとのショップの依頼 = %d 件, want 0 件", n)
		}
	})

	t.Run("同じショップの複数のバーガーへの書き込みは、ショップの依頼が 1 件にまとまり、ショップの集計の再計算は 1 回で済む", func(t *testing.T) {
		alice := w.user(t, "coalesce-alice")
		b1, b2, b3 := w.burger(t, "Coalesce 1"), w.burger(t, "Coalesce 2"), w.burger(t, "Coalesce 3")
		w.review(t, alice, b1, 5, "a")
		w.review(t, alice, b2, 3, "b")
		w.review(t, alice, b3, 4, "c")

		if n := w.shopRequests(t, w.shop); n != 1 {
			t.Fatalf("ショップの依頼 = %d 件, want 1 件(3 つのバーガーへの書き込みからの依頼が 1 件にまとまる)", n)
		}
		if n, err := w.worker.RunOnce(ctx); err != nil || n != 3 {
			t.Fatalf("バーガーのワーカーの RunOnce = (%d, %v), want (3, nil)(バーガーごとに 3 件)", n, err)
		}
		if n, err := statsworkertest.NewShopWorker(conn, infra.SystemClock{}).RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("ショップのワーカーの RunOnce = (%d, %v), want (1, nil)(ショップの再計算は 1 回)", n, err)
		}
		if got := w.shopStat(t, w.shop); got.ReviewCount < 3 {
			t.Errorf("集計 = %+v, want 3 つのバーガーのレビューをまとめた件数(3 以上)", got)
		}
	})

	t.Run("ショップに紐づかないバーガーへの投稿は、ショップの依頼を積まず、失敗もしない", func(t *testing.T) {
		alice := w.user(t, "orphan-alice")
		orphan := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Orphan Burger") // どのショップにも紐づけない
		if _, err := conn.Exec(ctx, `INSERT INTO reviews (rating, user_id, burger_id) VALUES (3, $1, $2)`, alice.ID, orphan); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO burger_stats_recalc_requests (burger_id) VALUES ($1)`, orphan); err != nil {
			t.Fatal(err)
		}
		var before int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM shop_stats_recalc_requests`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		w.settle(t)
		var after int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM shop_stats_recalc_requests`).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Errorf("ショップの依頼 = %d 件 → %d 件, want 増えない", before, after)
		}
		if req, ok := dbtest.FetchRecalcRequest(ctx, t, conn, orphan); ok {
			t.Errorf("バーガーの依頼が残っている(失敗している): %+v", req)
		}
		if got := dbtest.RequireConsistentStats(ctx, t, conn, orphan); got.ReviewCount != 1 {
			t.Errorf("バーガーの統計 = %+v, want 件数 1", got)
		}
	})
}

// TestShopStatsFollowsReviewChanges は、レビューが増えた・編集された・削除された・書いた利用者が退会したあとに、
// ワーカーを動かすと、ショップの集計が変わることを、本物の usecase の書き込みで確かめる。
func TestShopStatsFollowsReviewChanges(t *testing.T) {
	w := newWorld(t)
	shopID := dbtest.InsertUUIDRow(w.ctx, t, w.conn, `INSERT INTO shops (name, status) VALUES ('Changes Grill', 1) RETURNING id`)
	w.shop = shopID
	burger := w.burger(t, "Changes Burger")
	other := w.burger(t, "Changes Other Burger")
	alice, bob, carol := w.user(t, "chg-alice"), w.user(t, "chg-bob"), w.user(t, "chg-carol")

	count := func(t *testing.T) (int64, float64) {
		t.Helper()
		w.settleAll(t)
		stat := w.shopStat(t, shopID)
		if stat.AverageRating == nil {
			return stat.ReviewCount, 0
		}
		return stat.ReviewCount, *stat.AverageRating
	}
	expect := func(t *testing.T, wantCount int64, wantAverage float64, why string) {
		t.Helper()
		if gotCount, gotAverage := count(t); gotCount != wantCount || gotAverage != wantAverage {
			t.Errorf("%s: 集計 = (件数 %d, 平均 %v), want (%d, %v)", why, gotCount, gotAverage, wantCount, wantAverage)
		}
	}

	aliceReview := w.review(t, alice, burger, 5, "a")
	bobReview := w.review(t, bob, other, 3, "b") // 別のバーガーでも、同じショップにまとまる
	expect(t, 2, 4, "2 件を投稿した(5 と 3。全員が新規の投稿者で同じ重みなので、平均は 4)")

	carolReview := w.review(t, carol, burger, 1, "c")
	expect(t, 3, 3, "3 件目を投稿した(5・3・1)")

	if _, err := w.reviews.Update(w.ctx, carol, carolReview.ID, 4, "changed", nil, nil); err != nil {
		t.Fatal(err)
	}
	expect(t, 3, 4, "carol が 1 を 4 に編集した")

	if err := w.reviews.Delete(w.ctx, bob, bobReview.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, 2, 4.5, "bob が削除した(5 と 4)")

	if err := w.users.Delete(w.ctx, alice, alice.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, 1, 4, "alice が退会した(退会した利用者のレビューは数えない。残るのは carol の 4)")
	_ = aliceReview

	if err := w.reviews.Delete(w.ctx, carol, carolReview.ID); err != nil {
		t.Fatal(err)
	}
	w.settleAll(t)
	if got := w.shopStat(t, shopID); got.ReviewCount != 0 || got.AverageRating != nil || got.PhotoKey != nil {
		t.Errorf("すべて消えたあとの集計 = %+v, want 件数 0・平均と写真なし(行は残る)", got)
	}
}

// TestShopStatsWeightedAggregation は、ショップの評価が、そのショップのすべてのバーガーのレビューを 1 つの集合として、
// バーガーのスコアと同じ重み付け(投稿者の信頼度 × 半減期 180 日の新しさ)で加重平均した、手で計算した値になることを、
// 実際のデータベースのデータで確かめる。削除済みのレビュー・退会した利用者のレビュー・別のショップのレビューは含まれない。
func TestShopStatsWeightedAggregation(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	shop := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO shops (name, status) VALUES ('Weighted Grill', 1) RETURNING id`)
	otherShop := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO shops (name, status) VALUES ('Weighted Other', 1) RETURNING id`)
	burger := func(name, shopID string) string {
		id := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, name)
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	b1, b2, elsewhere := burger("Weighted 1", shop), burger("Weighted 2", shop), burger("Weighted Elsewhere", otherShop)
	user := func(name string) string {
		return dbtest.InsertUserRow(ctx, t, conn, insertUser, name+"@example.com", name, false)
	}
	newcomerA, newcomerB, veteran, gone := user("weight-a"), user("weight-b"), user("weight-vet"), user("weight-gone")
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	review := func(userID, burgerID string, rating int, age time.Duration, photo string, discarded bool) string {
		t.Helper()
		var photoKey, discardedAt any
		if photo != "" {
			photoKey = photo
		}
		created := now.Add(-age)
		if discarded {
			discardedAt = created
		}
		return dbtest.InsertUUIDRow(ctx, t, conn,
			`INSERT INTO reviews (rating, user_id, burger_id, photo_key, discarded_at, created_at) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			rating, userID, burgerID, photoKey, discardedAt, created)
	}
	const day = 24 * time.Hour

	// このショップの対象のレビュー: 新規の投稿者(信頼度 0.5)の 5 点(新しい)と 1 点(180 日前)、10 件以上でばらつきのある
	// 投稿者(信頼度 0.9)の 4 点(新しい)。バーガーは 2 つにまたがる。
	review(newcomerA, b1, 5, 0, "reviews/newest.jpg", false)
	old := review(newcomerB, b2, 1, 180*day, "reviews/old.jpg", false)
	review(veteran, b1, 4, 0, "", false)
	// 対象外: 削除済み(写真つきで最も新しい)・退会した利用者・別のショップ(写真つきで最も新しい)。
	review(newcomerA, b1, 1, 0, "reviews/discarded-newer.jpg", true)
	review(gone, b2, 1, 0, "reviews/gone-newer.jpg", false)
	review(newcomerA, elsewhere, 2, 0, "reviews/other-shop-newer.jpg", false) // 新規の投稿者の 2 件目(履歴は 2 件で、信頼度は 0.5 のまま)
	// veteran の履歴を 10 件にする(このレビュー 1 件 + 別のショップの 9 件。評価にばらつきがあり、信頼度は 0.9)。
	for i, rating := range []int{1, 2, 3, 5, 1, 2, 3, 4, 5} {
		review(veteran, elsewhere, rating, time.Duration(i+1)*day, "", false)
	}

	settleAt := func(at time.Time) {
		t.Helper()
		if _, err := conn.Exec(ctx, `INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)
			ON CONFLICT (shop_id) DO UPDATE SET version = nextval('shop_stats_recalc_requests_version_seq'), attempts = 0, next_attempt_at = NULL, last_error = NULL`, shop); err != nil {
			t.Fatal(err)
		}
		statsworkertest.SettleShops(ctx, t, statsworkertest.NewShopWorker(conn, uowtest.Clock{T: at}))
	}

	t.Run("平均は、重み 0.5(新しい 5 点)・0.25(180 日前の 1 点)・0.9(新しい 4 点)の加重平均 (5×0.5+1×0.25+4×0.9)/1.65 = 3.85 → 3.8 で、単純平均 3.3 と違う", func(t *testing.T) {
		settleAt(now)
		got := w.shopStat(t, shop)
		if got.ReviewCount != 3 {
			t.Errorf("件数 = %d, want 3(削除済み・退会した利用者・別のショップのレビューを除く。2 つのバーガーをまとめる)", got.ReviewCount)
		}
		if got.AverageRating == nil || *got.AverageRating != 3.8 {
			t.Errorf("平均 = %v, want 3.8(単純平均なら 3.3)", got.AverageRating)
		}
		if got.PhotoKey == nil || *got.PhotoKey != "reviews/newest.jpg" {
			t.Errorf("写真 = %v, want reviews/newest.jpg(対象外のレビューの新しい写真は使わない)", got.PhotoKey)
		}
		if !got.CalculatedAt.Equal(now) {
			t.Errorf("計算時刻 = %v, want %v", got.CalculatedAt, now)
		}
	})

	t.Run("新しさの重みが効く: 180 日前の 1 点が新しくなると、重みが 0.25 から 0.5 に上がり、平均は (2.5+0.5+3.6)/1.9 = 3.47 → 3.5 に下がる", func(t *testing.T) {
		if _, err := conn.Exec(ctx, `UPDATE reviews SET created_at = $2 WHERE id = $1`, old, now); err != nil {
			t.Fatal(err)
		}
		settleAt(now)
		if got := w.shopStat(t, shop); got.AverageRating == nil || *got.AverageRating != 3.5 {
			t.Errorf("平均 = %v, want 3.5", got.AverageRating)
		}
	})

	t.Run("時間が経っても、同じレビューの集合の平均は変わらない: 新しさの重みは指数関数の減衰で、全員に共通の係数がかかるだけなので、180 日後に再計算しても 3.8 のままである", func(t *testing.T) {
		// 古い 1 点を 180 日前に戻す(重み 0.25)。180 日後の再計算では、新しい 2 件は重みが 0.25・0.45、古い 1 件は 0.125 になる。
		// (5×0.25 + 1×0.125 + 4×0.45)/(0.25+0.125+0.45) = 3.175/0.825 = 3.85 → 3.8。重みの比が変わらないので、平均も変わらない。
		// 平均が変わるのは、あとから新しいレビューが入ったときだけである(新しいレビューの重みが、相対的に大きくなる)。
		if _, err := conn.Exec(ctx, `UPDATE reviews SET created_at = $2 WHERE id = $1`, old, now.Add(-180*day)); err != nil {
			t.Fatal(err)
		}
		settleAt(now.Add(180 * day))
		if got := w.shopStat(t, shop); got.AverageRating == nil || *got.AverageRating != 3.8 {
			t.Errorf("平均 = %v, want 3.8(時間が経っても変わらない)", got.AverageRating)
		}
	})
}

// TestShopStatsConvergesAfterConcurrentWrite は、ショップの集計の再計算の最中に、別の接続から新しいレビューが確定
// しても、依頼が消えずに残り、次の再計算で最新の集計に収束することを確かめる。再計算は、依頼の行も、ショップの行を
// 待たせる強いロックも取らないので、書き込みも、その再計算の依頼の登録も、再計算に待たされない。
func TestShopStatsConvergesAfterConcurrentWrite(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	pool := w.pool(t)
	otherReviews := usecase.NewReviews(query.NewReviewQuery(pool), uow.New(pool), w.recalc, w.shopRecalc, storage.NewDisk(t.TempDir(), "/photos"), infra.SystemClock{})

	alice, bob := w.user(t, "conv-alice"), w.user(t, "conv-bob")
	b1, b2 := w.burger(t, "Converge 1"), w.burger(t, "Converge 2")
	w.review(t, alice, b1, 5, "first")
	w.settle(t)
	taken, ok := dbtest.FetchShopRecalcRequest(ctx, t, conn, w.shop)
	if !ok {
		t.Fatal("ショップの依頼がない")
	}

	var injected bool
	worker := w.hookedShopWorker(infra.SystemClock{}, 5, func(ctx context.Context, shopID string) error {
		if injected {
			return nil
		}
		injected = true
		// ワーカーは、ショップの行をロックして、元データ(alice の 1 件)を読み終えた。この時点で、別の接続から、
		// bob の投稿を確定させ、そのバーガーの統計の再計算(ショップの依頼の登録つき)まで済ませる。
		// 依頼の登録は、ショップの行を更新せず、依頼の行も待たないので、再計算に待たされない。
		writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if _, err := otherReviews.Create(writeCtx, bob, w.shop, b2, "", 3, "second", nil, nil); err != nil {
			return err
		}
		if _, err := statsworkertest.NewWorker(pool, infra.SystemClock{}).RunOnce(writeCtx); err != nil {
			return err
		}
		return nil
	})
	if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("ショップのワーカーの RunOnce = (%d, %v), want (1, nil)", n, err)
	}
	if !injected {
		t.Fatal("再計算の途中で、別の接続の書き込みを入れられなかった")
	}
	// 古い元データ(alice の 1 件)の集計は保存されるが、依頼は残る。
	if got := w.shopStat(t, w.shop); got.ReviewCount != 1 {
		t.Errorf("再計算の直後の集計 = %+v, want 古い元データの件数 1(bob の投稿は、次の再計算で反映される)", got)
	}
	left, ok := dbtest.FetchShopRecalcRequest(ctx, t, conn, w.shop)
	if !ok {
		t.Fatal("再計算の最中に登録された依頼が、消えてしまった(bob の投稿が、集計に反映されなくなる)")
	}
	if left.Version <= taken.Version {
		t.Errorf("残った依頼の version = %d, want 取り出したときの %d より大きい", left.Version, taken.Version)
	}

	w.settleShops(t)
	if got := w.shopStat(t, w.shop); got.ReviewCount != 2 {
		t.Errorf("次の再計算のあとの集計 = %+v, want 2 件の投稿を反映した件数 2", got)
	}
	if n := w.shopRequests(t, w.shop); n != 0 {
		t.Errorf("次の再計算のあとの依頼 = %d 件, want 0 件", n)
	}
}

// TestShopStatsWorkerFailure は、ショップの集計の再計算に失敗した依頼の扱いを、実際のデータベースの行で確かめる。
// 失敗は、失敗の回数・次の再試行の時刻・理由として行に残り、待ち時間が過ぎるまで再試行されず、上限の回数で打ち切られる。
func TestShopStatsWorkerFailure(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "fail-alice")
	burger := w.burger(t, "Failing Shop Burger")
	w.review(t, alice, burger, 5, "x")
	w.settle(t)

	failing := func(clock usecase.Clock, maxAttempts int, failures *int) *usecase.ShopStatsWorker {
		return w.hookedShopWorker(clock, maxAttempts, func(context.Context, string) error {
			if *failures > 0 {
				*failures--
				return errors.New("reading shop review facts failed")
			}
			return nil
		})
	}

	t.Run("失敗した依頼は、失敗の回数・次の再試行の時刻・理由が残り、待ち時間の間は再試行されず、過ぎたら再試行されて成功する", func(t *testing.T) {
		clock := newSteppingClock()
		failures := 1
		worker := failing(clock, 5, &failures)
		if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
			t.Fatalf("RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		if _, ok := dbtest.FetchShopStats(ctx, t, conn, w.shop); ok {
			t.Fatal("失敗したのに、集計の行ができている")
		}
		req, ok := dbtest.FetchShopRecalcRequest(ctx, t, conn, w.shop)
		if !ok || req.Attempts != 1 || req.NextAttemptAt == nil || !req.NextAttemptAt.Equal(clock.Now().Add(2*time.Second)) {
			t.Fatalf("失敗した依頼 = %+v (ok %v), want 失敗 1 回・次の再試行は 2 秒後", req, ok)
		}
		if req.LastError == nil || !strings.Contains(*req.LastError, "reading shop review facts failed") {
			t.Errorf("失敗の理由 = %v, want 元のエラーの文言を含む", req.LastError)
		}
		clock.Advance(time.Second)
		if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
			t.Fatalf("待ち時間の間の RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		clock.Advance(2 * time.Second)
		if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("待ち時間が過ぎたあとの RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if got := w.shopStat(t, w.shop); got.ReviewCount != 1 {
			t.Errorf("再試行のあとの集計 = %+v, want 件数 1", got)
		}
		if n := w.shopRequests(t, w.shop); n != 0 {
			t.Errorf("再試行に成功したあとの依頼 = %d 件, want 0 件", n)
		}
	})

	t.Run("失敗が続くと、上限の回数で打ち切られ、行は残り、新しい依頼(バーガーの書き込み)で最初からやり直せる", func(t *testing.T) {
		logs := captureSlog(t)
		clock := newSteppingClock()
		if _, err := conn.Exec(ctx, `INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)
			ON CONFLICT (shop_id) DO UPDATE SET attempts = 0, next_attempt_at = NULL, last_error = NULL`, w.shop); err != nil {
			t.Fatal(err)
		}
		failures := 1000
		worker := failing(clock, 3, &failures)
		for attempt, wantDelay := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second} {
			if _, err := worker.RunOnce(ctx); err != nil {
				t.Fatalf("%d 回目の RunOnce: %v", attempt+1, err)
			}
			req, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, w.shop)
			if req.Attempts != attempt+1 || req.NextAttemptAt == nil || !req.NextAttemptAt.Equal(clock.Now().Add(wantDelay)) {
				t.Fatalf("%d 回目の失敗のあとの依頼 = %+v, want 失敗 %d 回・次の再試行は %v 後", attempt+1, req, attempt+1, wantDelay)
			}
			clock.Advance(wantDelay)
		}
		if !strings.Contains(logs.String(), "shop stats recalculation gave up") {
			t.Errorf("ログ = %q, want 打ち切りの error を含む", logs)
		}
		if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
			t.Fatalf("打ち切り後の RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		if n := w.shopRequests(t, w.shop); n != 1 {
			t.Fatalf("打ち切り後の依頼 = %d 件, want 1 件(行は残る)", n)
		}
		// バーガーの書き込みがあれば、バーガーのワーカーが、ショップの依頼を積み直し、最初からやり直す。
		w.review(t, w.user(t, "fail-bob"), burger, 3, "again")
		w.settle(t)
		if req, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, w.shop); req.Attempts != 0 || req.NextAttemptAt != nil {
			t.Errorf("積み直した依頼 = %+v, want 失敗の記録が消えている", req)
		}
		failures = 0
		if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("やり直しの RunOnce = (%d, %v), want (1, nil)", n, err)
		}
	})
}

// TestShopStatsWorkersConcurrently は、複数のインスタンスのワーカーと、複数の書き込みが同時に動いても、
// 最後に、すべてのレビューを反映した集計に収束することを確かめる(-race で、データ競合も検出する)。
func TestShopStatsWorkersConcurrently(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	pool := w.pool(t)
	otherReviews := usecase.NewReviews(query.NewReviewQuery(pool), uow.New(pool), w.recalc, w.shopRecalc, storage.NewDisk(t.TempDir(), "/photos"), infra.SystemClock{})

	const writers, perWriter = 4, 5
	burgers := make([]string, writers)
	users := make([]domain.User, writers)
	for i := range burgers {
		burgers[i] = w.burger(t, fmt.Sprintf("Concurrent Shop Burger %d", i))
		users[i] = w.user(t, fmt.Sprintf("concurrent-%d", i))
	}

	stop := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ { // 2 つのインスタンスのワーカー
		workers.Add(1)
		go func() {
			defer workers.Done()
			burgerWorker, shopWorker := statsworkertest.NewWorker(pool, infra.SystemClock{}), statsworkertest.NewShopWorker(pool, infra.SystemClock{})
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := burgerWorker.RunOnce(context.Background()); err != nil {
					t.Errorf("バーガーのワーカーの RunOnce: %v", err)
				}
				if _, err := shopWorker.RunOnce(context.Background()); err != nil {
					t.Errorf("ショップのワーカーの RunOnce: %v", err)
				}
			}
		}()
	}
	var writes sync.WaitGroup
	for i := 0; i < writers; i++ {
		writes.Add(1)
		go func() {
			defer writes.Done()
			for n := 0; n < perWriter; n++ {
				if _, err := otherReviews.Create(context.Background(), users[i], w.shop, burgers[i], "", 1+(n+i)%5, "concurrent", nil, nil); err != nil {
					t.Errorf("レビューの投稿: %v", err)
					return
				}
			}
		}()
	}
	writes.Wait()
	close(stop)
	workers.Wait()

	w.settleAll(t)
	got := w.shopStat(t, w.shop)
	if got.ReviewCount != writers*perWriter {
		t.Errorf("件数 = %d, want %d(同時の書き込みを、すべて反映している)", got.ReviewCount, writers*perWriter)
	}
	// 最後の集計は、保存されているレビューから、同じ計算で求め直した値と一致する(最後の再計算が、すべてのレビューを読んでいる)。
	statsQuery := query.NewShopStatsQuery(conn)
	facts, err := statsQuery.ListShopReviewFacts(ctx, w.shop)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.CalculateShopStat(w.shop, facts, got.CalculatedAt)
	if got.AverageRating == nil || want.AverageRating == nil || *got.AverageRating != *want.AverageRating {
		t.Errorf("平均 = %v, want 保存されているレビューから求め直した %v", got.AverageRating, want.AverageRating)
	}
	if n := w.shopRequests(t, w.shop); n != 0 {
		t.Errorf("依頼 = %d 件, want 0 件", n)
	}
}

// TestShopStatsMatchesDirectAggregation は、ワーカーが保存した集計が、変更前の作り(読み取りのたびに、レビューから
// 直接集計する SQL)と、件数と写真で一致し、平均が、バーガーのスコアの計算器を、ショップのすべてのレビューにかけた
// 値(独立に求めた投稿者の履歴つき)と一致することを、複数のショップ・バーガーにまたがる、さまざまなデータで確かめる。
// データには、削除済みのレビュー・退会した利用者・複数のショップにあるバーガー・写真の有無・同じ時刻のレビューを含める。
func TestShopStatsMatchesDirectAggregation(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(20260922))

	const shopCount, burgerCount, userCount, reviewCount = 6, 14, 9, 120
	shops := make([]string, shopCount)
	for i := range shops {
		shops[i] = dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO shops (name, status) VALUES ($1, 1) RETURNING id`, fmt.Sprintf("Shop %d", i))
	}
	burgers := make([]string, burgerCount)
	for i := range burgers {
		burgers[i] = dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, fmt.Sprintf("Burger %d", i))
		// 1 つのバーガーは、1〜2 つのショップに紐づく(複数のショップにあるバーガーを含む)。
		linked := map[int]bool{i % shopCount: true}
		if i%3 == 0 {
			linked[(i+1)%shopCount] = true
		}
		for shop := range linked {
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shops[shop], burgers[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	users := make([]string, userCount)
	for i := range users {
		users[i] = dbtest.InsertUserRow(ctx, t, conn, insertUser, fmt.Sprintf("agg-%d@example.com", i), fmt.Sprintf("agg-%d", i), false)
	}
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = ANY($1::uuid[])`, []string{users[userCount-1], users[userCount-2]}); err != nil {
		t.Fatal(err)
	}
	sameTime := now.Add(-30 * 24 * time.Hour)
	for i := 0; i < reviewCount; i++ {
		var photo, discardedAt any
		if rng.Intn(10) < 3 {
			photo = fmt.Sprintf("reviews/agg-%d.jpg", i)
		}
		created := now.Add(-time.Duration(rng.Intn(400*24)) * time.Hour)
		if i%11 == 0 {
			created = sameTime // 同じ時刻のレビュー(写真の選び方の順序を、id で決める)
		}
		if rng.Intn(100) < 8 {
			discardedAt = created
		}
		if _, err := conn.Exec(ctx,
			`INSERT INTO reviews (rating, user_id, burger_id, photo_key, discarded_at, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			1+rng.Intn(5), users[rng.Intn(userCount)], burgers[rng.Intn(burgerCount)], photo, discardedAt, created); err != nil {
			t.Fatal(err)
		}
	}

	// 変更前の作り(読み取りのたびの直接の集計)と同じ内容の SQL。ワーカーの実装とは独立に、テストが持つ。
	type direct struct {
		count int64
		photo *string
	}
	directOf := func(shopID string) direct {
		t.Helper()
		var d direct
		if err := conn.QueryRow(ctx, `
			SELECT count(*) FROM reviews r
			JOIN shops_burgers sb ON sb.burger_id = r.burger_id
			JOIN users u ON u.id = r.user_id
			WHERE sb.shop_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL`, shopID).Scan(&d.count); err != nil {
			t.Fatal(err)
		}
		err := conn.QueryRow(ctx, `
			SELECT r.photo_key FROM reviews r
			JOIN shops_burgers sb ON sb.burger_id = r.burger_id
			JOIN users u ON u.id = r.user_id
			WHERE sb.shop_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL AND r.photo_key IS NOT NULL
			ORDER BY r.created_at DESC, r.id DESC LIMIT 1`, shopID).Scan(&d.photo)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		return d
	}
	// バーガーのスコアの計算器を、ショップのすべてのレビューにかけた値。投稿者の履歴は、別の SQL(配列の集約)で求める。
	weightedOf := func(shopID string) (float64, bool) {
		t.Helper()
		rows, err := conn.Query(ctx, `
			SELECT r.rating::float8, r.created_at,
			       (SELECT array_agg(r2.rating::float8) FROM reviews r2 WHERE r2.user_id = r.user_id AND r2.discarded_at IS NULL)
			FROM reviews r
			JOIN shops_burgers sb ON sb.burger_id = r.burger_id
			JOIN users u ON u.id = r.user_id
			WHERE sb.shop_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL`, shopID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var facts []domain.ReviewFact
		for rows.Next() {
			var fact domain.ReviewFact
			var history []float64
			if err := rows.Scan(&fact.Rating, &fact.CreatedAt, &history); err != nil {
				t.Fatal(err)
			}
			fact.ReviewerHistory = domain.ReviewerHistory{Ratings: history}
			facts = append(facts, fact)
		}
		if len(facts) == 0 {
			return 0, false
		}
		return domain.CalculateBurgerScore(facts, now).WeightedAverage, true
	}

	for _, shop := range shops {
		if _, err := conn.Exec(ctx, `INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)`, shop); err != nil {
			t.Fatal(err)
		}
	}
	statsworkertest.SettleShops(ctx, t, statsworkertest.NewShopWorker(conn, uowtest.Clock{T: now}))

	checked := 0
	for i, shop := range shops {
		want := directOf(shop)
		got := w.shopStat(t, shop)
		if got.ReviewCount != want.count {
			t.Errorf("ショップ %d: 件数 = %d, 直接の集計 = %d", i, got.ReviewCount, want.count)
		}
		if (got.PhotoKey == nil) != (want.photo == nil) || (got.PhotoKey != nil && *got.PhotoKey != *want.photo) {
			t.Errorf("ショップ %d: 写真 = %v, 直接の集計 = %v", i, got.PhotoKey, want.photo)
		}
		weighted, ok := weightedOf(shop)
		switch {
		case !ok && got.AverageRating != nil:
			t.Errorf("ショップ %d: レビューがないのに平均 %v がある", i, *got.AverageRating)
		case ok && (got.AverageRating == nil || math.Abs(*got.AverageRating-weighted) > 0.05+1e-9):
			t.Errorf("ショップ %d: 平均 = %v, バーガーのスコアの計算器 = %v(差は丸めの 0.05 以内のはず)", i, got.AverageRating, weighted)
		}
		if want.count > 0 {
			checked++
		}
	}
	if checked < shopCount-1 {
		t.Fatalf("レビューのあるショップが %d しかない(データが偏っていて、比較の意味が薄い)", checked)
	}
}
