// DB 単体のレイテンシを、読みと書きの両方で測る。
// アプリの HTTP 層を挟まず pgx で直接叩くので、DB とネットワークの往復だけが出る。
//
//	usage: DATABASE_URL=... ITERATIONS=200 go run .
//
// 測るのは backend-go が実際に使う形のクエリ。書き込みは測り終えたら消すので、
// 候補間でデータ量がずれない。
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sample struct {
	name string
	ds   []time.Duration
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	n := 200
	if v := os.Getenv("ITERATIONS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			n = p
		}
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	// 外部キーに使う既存の行を 1 件ずつ取る。
	var userID, burgerID, shopID string
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE discarded_at IS NULL LIMIT 1`).Scan(&userID); err != nil {
		return fmt.Errorf("users が空: %w", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM burgers LIMIT 1`).Scan(&burgerID); err != nil {
		return fmt.Errorf("burgers が空: %w", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM shops LIMIT 1`).Scan(&shopID); err != nil {
		return fmt.Errorf("shops が空: %w", err)
	}

	var results []sample

	// ---------- 読み ----------
	results = append(results, measure("往復のみ (SELECT 1)", n, true, func() error {
		var one int
		return pool.QueryRow(ctx, `SELECT 1`).Scan(&one)
	}))

	results = append(results, measure("主キー 1 件 (shops)", n, true, func() error {
		var id, name string
		return pool.QueryRow(ctx, `SELECT id, name FROM shops WHERE id = $1`, shopID).Scan(&id, &name)
	}))

	results = append(results, measure("一覧 20 件 (shops)", n, true, func() error {
		rows, err := pool.Query(ctx, `SELECT id, name, status FROM shops WHERE status = 1 ORDER BY created_at DESC LIMIT 20`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
		}
		return rows.Err()
	}))

	results = append(results, measure("結合 20 件 (reviews + burgers + stats)", n, true, func() error {
		rows, err := pool.Query(ctx, `
			SELECT r.id, r.rating, r.comment, b.name, bs.weighted_score
			FROM reviews r
			JOIN burgers b ON b.id = r.burger_id
			LEFT JOIN burger_stats bs ON bs.burger_id = b.id
			WHERE r.discarded_at IS NULL
			ORDER BY r.created_at DESC
			LIMIT 20`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
		}
		return rows.Err()
	}))

	results = append(results, measure("集計 (バーガー別の平均評価)", n, true, func() error {
		var cnt int64
		return pool.QueryRow(ctx, `
			SELECT count(*) FROM (
				SELECT burger_id, avg(rating) FROM reviews
				WHERE discarded_at IS NULL GROUP BY burger_id
			) t`).Scan(&cnt)
	}))

	// ---------- 書き ----------
	ids := make([]string, 0, n)
	results = append(results, measure("INSERT (reviews 1 行)", n, false, func() error {
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO reviews (rating, comment, user_id, burger_id)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			4, "dbops-bench", userID, burgerID).Scan(&id)
		if err == nil {
			ids = append(ids, id)
		}
		return err
	}))

	i := 0
	results = append(results, measure("UPDATE (reviews 1 行)", len(ids), false, func() error {
		id := ids[i]
		i++
		_, err := pool.Exec(ctx, `UPDATE reviews SET rating = $1, comment = $2, updated_at = now() WHERE id = $3`,
			5, "dbops-bench-updated", id)
		return err
	}))

	// 実際の書き込み経路。レビューの作成と再計算依頼の登録を 1 トランザクションで行う。
	txIDs := make([]string, 0, n)
	results = append(results, measure("トランザクション (INSERT + 再計算依頼)", n, false, func() error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var id string
			if err := tx.QueryRow(ctx, `
				INSERT INTO reviews (rating, comment, user_id, burger_id)
				VALUES ($1, $2, $3, $4) RETURNING id`,
				3, "dbops-bench-tx", userID, burgerID).Scan(&id); err != nil {
				return err
			}
			txIDs = append(txIDs, id)
			_, err := tx.Exec(ctx, `
				INSERT INTO burger_stats_recalc_requests (burger_id) VALUES ($1)
				ON CONFLICT (burger_id) DO UPDATE SET version = nextval('burger_stats_recalc_requests_version_seq'), updated_at = now()`,
				burgerID)
			return err
		})
	}))

	j := 0
	all := append(append([]string{}, ids...), txIDs...)
	results = append(results, measure("DELETE (reviews 1 行)", len(all), false, func() error {
		id := all[j]
		j++
		_, err := pool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, id)
		return err
	}))

	// 後片付け(取りこぼし)
	_, _ = pool.Exec(ctx, `DELETE FROM reviews WHERE comment LIKE 'dbops-bench%'`)

	printTable(results)
	return nil
}

func measure(name string, n int, warm bool, op func() error) sample {
	s := sample{name: name, ds: make([]time.Duration, 0, n)}
	// 読みは 1 回捨てて、接続の確立や prepared statement の登録を計測から外す。
	// 書きは行を 1 つ余計に作り、後続の添字とずれるので捨てない。
	if warm {
		_ = op()
	}
	for k := 0; k < n; k++ {
		t0 := time.Now()
		if err := op(); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", name, err)
			break
		}
		s.ds = append(s.ds, time.Since(t0))
	}
	return s
}

func printTable(rs []sample) {
	fmt.Println("| 操作 | n | 平均 | 標準偏差 | p50 | p90 | p95 | p99 | 最小 | 最大 |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range rs {
		if len(r.ds) == 0 {
			fmt.Printf("| %s | 0 | 失敗 | | | | | | | |\n", r.name)
			continue
		}
		d := append([]time.Duration{}, r.ds...)
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		fmt.Printf("| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			r.name, len(d), ms(mean(d)), ms(stddev(d)),
			ms(pct(d, 0.50)), ms(pct(d, 0.90)), ms(pct(d, 0.95)), ms(pct(d, 0.99)),
			ms(d[0]), ms(d[len(d)-1]))
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }

func mean(d []time.Duration) time.Duration {
	var t time.Duration
	for _, x := range d {
		t += x
	}
	return t / time.Duration(len(d))
}

func stddev(d []time.Duration) time.Duration {
	m := float64(mean(d))
	var sum float64
	for _, x := range d {
		diff := float64(x) - m
		sum += diff * diff
	}
	return time.Duration(math.Sqrt(sum / float64(len(d))))
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}
