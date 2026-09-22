package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/statsworkertest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
)

// TestDevPasswordSatisfiesPolicy は、seed のパスワードが signup と同じ強度ルールを
// 満たすことを固定する。規則が変わったとき、開発用 fixture だけが現行の規則で
// 作れない値のまま残るのを防ぐ。
func TestDevPasswordSatisfiesPolicy(t *testing.T) {
	if issues := domain.PasswordIssues(devPassword); len(issues) != 0 {
		t.Fatalf("devPassword violates the password policy: %q", domain.Texts(domain.LangEN, issues))
	}
}

// TestSeedRequestsShopStatsRecalculation は、seed が、ショップの集計の規則を複製せず、ショップの再計算の依頼を積むだけに
// し、ワーカーが動くと、seed されたショップの集計(件数・重み付きの平均)が埋まること、seed を続けて 2 回実行しても、
// 失敗せず、集計が変わらないことを確かめる。
func TestSeedRequestsShopStatsRecalculation(t *testing.T) {
	ctx := context.Background()
	conn, url := dbtest.New(t)
	t.Setenv("DATABASE_URL", url)

	if err := run(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var withBurgers, requests, stats int
	if err := conn.QueryRow(ctx, `SELECT
		(SELECT count(DISTINCT shop_id) FROM shops_burgers),
		(SELECT count(*) FROM shop_stats_recalc_requests),
		(SELECT count(*) FROM shop_stats)`).Scan(&withBurgers, &requests, &stats); err != nil {
		t.Fatal(err)
	}
	if withBurgers == 0 || requests != withBurgers || stats != 0 {
		t.Fatalf("seed 直後: バーガーのあるショップ = %d, 依頼 = %d, 集計 = %d, want 依頼 = バーガーのあるショップの数・集計はまだない(あとからワーカーが計算する)", withBurgers, requests, stats)
	}

	statsworkertest.SettleShops(ctx, t, statsworkertest.NewShopWorker(conn, uowtest.Clock{}))
	var reviewed int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM shop_stats WHERE review_count > 0 AND average_rating IS NOT NULL`).Scan(&reviewed); err != nil {
		t.Fatal(err)
	}
	if reviewed == 0 {
		t.Fatal("ワーカーが動いたあとも、レビューのあるショップの集計がない")
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM shop_stats_recalc_requests`).Scan(&requests); err != nil || requests != 0 {
		t.Errorf("ワーカーが動いたあとの依頼 = %d (err %v), want 0", requests, err)
	}

	before := fetchShopStats(ctx, t, conn)
	if err := run(ctx); err != nil {
		t.Fatalf("2 回目の seed: %v", err)
	}
	statsworkertest.SettleShops(ctx, t, statsworkertest.NewShopWorker(conn, uowtest.Clock{}))
	if after := fetchShopStats(ctx, t, conn); after != before {
		t.Errorf("2 回目の seed のあとの集計が変わった:\n before %s\n after  %s", before, after)
	}
}

func fetchShopStats(ctx context.Context, t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var out string
	if err := conn.QueryRow(ctx,
		`SELECT coalesce(string_agg(shop_id::text || ':' || review_count || ':' || coalesce(average_rating::text, '-') || ':' || coalesce(photo_key, '-'), ',' ORDER BY shop_id), '') FROM shop_stats`).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
