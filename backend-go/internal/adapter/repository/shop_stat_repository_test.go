package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

func newShop(ctx context.Context, t *testing.T, conn *pgx.Conn, name string) string {
	t.Helper()
	return dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO shops (name, status) VALUES ($1, 1) RETURNING id`, name)
}

func float64Ptr(f float64) *float64 { return &f }

func stringPtr(s string) *string { return &s }

// TestShopStatRepository は、ショップの集計の保存と、行のロックを、実際の PostgreSQL に対して検証する。
func TestShopStatRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	calculatedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	t.Run("集計を保存すると行ができ、もう一度保存すると、同じ行が新しい値で置き換わる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Stat Grill")
		first := domain.ShopStat{ShopID: shop, ReviewCount: 3, AverageRating: float64Ptr(4.3), PhotoKey: stringPtr("reviews/a.jpg"), CalculatedAt: calculatedAt}
		if err := repo.UpdateShopStat(ctx, first); err != nil {
			t.Fatalf("保存: %v", err)
		}
		got, ok := dbtest.FetchShopStats(ctx, t, conn, shop)
		if !ok || got.ReviewCount != 3 || got.AverageRating == nil || *got.AverageRating != 4.3 || got.PhotoKey == nil || *got.PhotoKey != "reviews/a.jpg" || !got.CalculatedAt.Equal(calculatedAt) {
			t.Fatalf("保存した集計 = %+v (ok %v), want %+v", got, ok, first)
		}
		second := domain.ShopStat{ShopID: shop, ReviewCount: 0, CalculatedAt: calculatedAt.Add(time.Minute)} // レビューがなくなった
		if err := repo.UpdateShopStat(ctx, second); err != nil {
			t.Fatalf("上書き: %v", err)
		}
		got, _ = dbtest.FetchShopStats(ctx, t, conn, shop)
		if got.ReviewCount != 0 || got.AverageRating != nil || got.PhotoKey != nil {
			t.Errorf("上書きした集計 = %+v, want 件数 0・平均と写真は NULL", got)
		}
		var rows int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM shop_stats WHERE shop_id = $1`, shop).Scan(&rows); err != nil || rows != 1 {
			t.Errorf("行の数 = %d (err %v), want 1", rows, err)
		}
	})

	t.Run("存在しないショップの集計は保存できない(外部キー)", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		err := repository.NewShopStatRepository(conn).UpdateShopStat(ctx, domain.ShopStat{ShopID: uid.N(999), ReviewCount: 1, AverageRating: float64Ptr(3), CalculatedAt: calculatedAt})
		if err == nil || !strings.Contains(err.Error(), "shop_stats_shop_id_fkey") {
			t.Errorf("err = %v, want 外部キー違反", err)
		}
	})

	t.Run("ショップの行をロックできる。ショップがないときは、ErrShopNotFound を返す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Lock Grill")
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		txRepo := repository.NewShopStatRepository(tx)
		if err := txRepo.LockShopStat(ctx, shop); err != nil {
			t.Fatalf("ロック: %v", err)
		}
		if err := txRepo.LockShopStat(ctx, shop); err != nil {
			t.Errorf("同じトランザクションが、ロックを取り直しても、待たされず成功する: %v", err)
		}
		if err := repo.LockShopStat(ctx, uid.N(999)); !errors.Is(err, domain.ErrShopNotFound) {
			t.Errorf("ないショップのロック: err = %v, want ErrShopNotFound", err)
		}
	})
}

// TestShopStatRepositoryRecalcRequests は、ショップの集計の再計算の依頼(shop_stats_recalc_requests)の登録・
// 比較つきの削除・失敗の記録を、実際の PostgreSQL に対して検証する(バーガーの依頼と同じ仕組み)。
func TestShopStatRepositoryRecalcRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()

	t.Run("登録すると、失敗の記録のない依頼が 1 行できる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Request Grill")
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatalf("登録: %v", err)
		}
		got, ok := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		if !ok || got.Attempts != 0 || got.NextAttemptAt != nil || got.LastError != nil {
			t.Errorf("登録した依頼 = %+v (ok %v), want 失敗の記録がない", got, ok)
		}
	})

	t.Run("同じショップに登録を重ねても 1 行のままで、version が進み、失敗の記録は消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Request Grill")
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatal(err)
		}
		first, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		failure := domain.NewRecalcFailure(3, context.DeadlineExceeded, time.Now())
		if ok, err := repo.UpdateShopStatRecalcFailure(ctx, shop, first.Version, failure); err != nil || !ok {
			t.Fatalf("失敗の記録 = (%v, %v), want (true, nil)", ok, err)
		}
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatal(err)
		}
		second, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		if second.Version <= first.Version || second.Attempts != 0 || second.NextAttemptAt != nil || second.LastError != nil {
			t.Errorf("重ねて登録した依頼 = %+v (前は %+v), want version が進み、失敗の記録が消えている", second, first)
		}
		var rows int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM shop_stats_recalc_requests WHERE shop_id = $1`, shop).Scan(&rows); err != nil || rows != 1 {
			t.Errorf("行の数 = %d (err %v), want 1(同じショップの依頼は 1 件にまとまる)", rows, err)
		}
	})

	t.Run("存在しないショップは登録できず、依頼の行も残らない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		if err := repository.NewShopStatRepository(conn).CreateShopStatRecalcRequest(ctx, uid.N(999)); err == nil {
			t.Fatal("存在しないショップの依頼を登録できてしまった")
		}
		if _, ok := dbtest.FetchShopRecalcRequest(ctx, t, conn, uid.N(999)); ok {
			t.Error("依頼の行が残っている")
		}
	})

	t.Run("取り出したときの version と同じときだけ依頼を消せる(消して作り直した依頼は、古い version に戻らない)", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Discard Grill")
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatal(err)
		}
		taken, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil { // 再計算の最中に、新しい依頼が入った
			t.Fatal(err)
		}
		if ok, err := repo.DiscardShopStatRecalcRequest(ctx, shop, taken.Version); err != nil || ok {
			t.Fatalf("古い version の削除 = (%v, %v), want (false, nil)", ok, err)
		}
		if _, ok := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop); !ok {
			t.Fatal("新しい依頼が消えた")
		}
		latest, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		if ok, err := repo.DiscardShopStatRecalcRequest(ctx, shop, latest.Version); err != nil || !ok {
			t.Fatalf("今の version の削除 = (%v, %v), want (true, nil)", ok, err)
		}
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatal(err)
		}
		recreated, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		if recreated.Version <= latest.Version {
			t.Errorf("作り直した依頼の version = %d, want %d より大きい(古い再計算が、新しい依頼を消さない)", recreated.Version, latest.Version)
		}
	})

	t.Run("失敗を記録すると、回数が増え、次の再試行の時刻と理由が入る。version が違う依頼には記録しない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Failure Grill")
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatal(err)
		}
		req, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		next := time.Now().Add(time.Minute).Truncate(time.Microsecond)
		if ok, err := repo.UpdateShopStatRecalcFailure(ctx, shop, req.Version, domain.RecalcFailure{NextAttemptAt: next, Reason: "boom"}); err != nil || !ok {
			t.Fatalf("失敗の記録 = (%v, %v), want (true, nil)", ok, err)
		}
		got, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		if got.Attempts != 1 || got.NextAttemptAt == nil || !got.NextAttemptAt.Equal(next) || got.LastError == nil || *got.LastError != "boom" || got.Version != req.Version {
			t.Errorf("記録した依頼 = %+v, want 回数 1・次の再試行 %v・理由 boom・version 不変", got, next)
		}
		if ok, err := repo.UpdateShopStatRecalcFailure(ctx, shop, req.Version+1000, domain.RecalcFailure{NextAttemptAt: next, Reason: "stale"}); err != nil || ok {
			t.Errorf("version が違う記録 = (%v, %v), want (false, nil)", ok, err)
		}
		if after, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop); after.Attempts != 1 {
			t.Errorf("version が違う記録で、回数が変わった: %+v", after)
		}
	})

	t.Run("文字数の上限ちょうどの理由は記録でき、上限を超える理由は拒否される", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewShopStatRepository(conn)
		shop := newShop(ctx, t, conn, "Length Grill")
		if err := repo.CreateShopStatRecalcRequest(ctx, shop); err != nil {
			t.Fatal(err)
		}
		req, _ := dbtest.FetchShopRecalcRequest(ctx, t, conn, shop)
		exact := strings.Repeat("あ", domain.MaxRecalcFailureReasonChars)
		if ok, err := repo.UpdateShopStatRecalcFailure(ctx, shop, req.Version, domain.RecalcFailure{NextAttemptAt: time.Now(), Reason: exact}); err != nil || !ok {
			t.Fatalf("上限ちょうどの理由 = (%v, %v), want (true, nil)", ok, err)
		}
		if _, err := repo.UpdateShopStatRecalcFailure(ctx, shop, req.Version, domain.RecalcFailure{NextAttemptAt: time.Now(), Reason: exact + "あ"}); err == nil {
			t.Error("上限を超える理由を記録できてしまった")
		}
	})
}
