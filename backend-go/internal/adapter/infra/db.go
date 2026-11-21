package infra

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// requiredTables は、API が正常に動作するために接続先スキーマに存在し、読み書き権限を
// 持っていなければならない全テーブルの一覧である。新しい CREATE TABLE マイグレーションを
// 追加したときに漏れがないことは db/migrations_test.go が自動で検査する。
var requiredTables = []string{
	"burger_stats",
	"burger_stats_recalc_requests",
	"burgers",
	"login_handoffs",
	"mail_deliveries",
	"oauth_grants",
	"oauth_token_sessions",
	"reviews",
	"shop_stats",
	"shop_stats_recalc_requests",
	"shops",
	"shops_burgers",
	"signup_verifications",
	"user_identities",
	"user_sessions",
	"users",
}

// RequiredTables は、HealthChecker が存在と読み書き権限を確かめる対象テーブル一覧の写しを返す。
func RequiredTables() []string {
	return slices.Clone(requiredTables)
}

// schemaCheckSQL は、1 回のカタログ問い合わせで以下をまとめて検証する:
//  1. current_schema() が利用可能であること（search_path が空、または USAGE 権限がない場合は NULL を CASE で安全に扱う）
//  2. requiredTables のすべての表が current_schema() に存在すること
//  3. 接続中の DB ロールがそれらの表すべてに SELECT/INSERT/UPDATE/DELETE 権限を持つこと
//  4. 後続の ALTER TABLE マイグレーションで追加した必須列が揃っていること
//  5. 退会後の再登録を支える有効な部分一意インデックス users_email_lower_key が適用済みであること
const schemaCheckSQL = `
WITH cur AS (
	SELECT current_schema() AS nsp
),
resolved_tables AS (
	SELECT
		t,
		CASE WHEN cur.nsp IS NOT NULL THEN to_regclass(format('%I.%I', cur.nsp, t)) END AS rel
	FROM unnest($1::text[]) AS t
	CROSS JOIN cur
),
resolved_columns AS (
	SELECT
		req.tbl,
		req.col,
		CASE WHEN cur.nsp IS NOT NULL THEN to_regclass(format('%I.%I', cur.nsp, req.tbl)) END AS rel
	FROM (VALUES
		('reviews', 'photo_key'),
		('reviews', 'visited_at'),
		('shops', 'closed_at'),
		('shops', 'map_url')
	) AS req(tbl, col)
	CROSS JOIN cur
)
SELECT
	COALESCE((SELECT nsp FROM cur), ''),
	COALESCE(array_agg(t ORDER BY t) FILTER (WHERE rel IS NULL), '{}'),
	COALESCE(array_agg(t ORDER BY t) FILTER (
		WHERE rel IS NOT NULL
		  AND NOT (
			has_table_privilege(rel, 'SELECT')
			AND has_table_privilege(rel, 'INSERT')
			AND has_table_privilege(rel, 'UPDATE')
			AND has_table_privilege(rel, 'DELETE')
		  )
	), '{}'),
	COALESCE((
		SELECT array_agg(rc.tbl || '.' || rc.col ORDER BY rc.tbl, rc.col)
		FROM resolved_columns rc
		WHERE rc.rel IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM pg_attribute a
			WHERE a.attrelid = rc.rel
			  AND a.attname = rc.col
			  AND a.attnum > 0
			  AND NOT a.attisdropped
		  )
	), '{}'),
	EXISTS (
		SELECT 1
		FROM pg_index ix
		JOIN pg_class c ON c.oid = ix.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN cur ON cur.nsp = n.nspname
		WHERE c.relname = 'users_email_lower_key'
		  AND ix.indisunique
		  AND ix.indisvalid
		  AND ix.indisready
		  AND ix.indpred IS NOT NULL
	)
FROM resolved_tables
`

// HealthDB は、死活監視と初回スキーマ検証に必要な最小の DB 操作を表す。
// *pgxpool.Pool と *pgx.Conn の両方が満たす。
type HealthDB interface {
	Ping(ctx context.Context) error
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// HealthChecker は、DB への疎通(Ping)に加えて、プロセス起動後の初回成功まで接続先 DB の
// スキーマ適用状態とテーブル権限を検証する。マイグレーション先(PROD_DATABASE_URL)と
// アプリの接続先(DATABASE_URL)が食い違った状態でデプロイされた場合に GET /up を 503 にし、
// トラフィック切り替え前の候補版チェックで確実に検知・遮断するために使う。
// 一度スキーマが確認できた後は通常の Ping のみを行い、定常時の DB 往復を増やさない。
type HealthChecker struct {
	db          HealthDB
	mu          sync.Mutex
	schemaReady atomic.Bool
}

// NewHealthChecker は db に対する HealthChecker を返す。
func NewHealthChecker(db HealthDB) *HealthChecker {
	if db == nil {
		panic("infra.NewHealthChecker: nil HealthDB")
	}
	return &HealthChecker{db: db}
}

// Ping は DB への疎通を確認し、まだスキーマ検証が済んでいなければ必須テーブル・列・インデックス・権限を確かめる。
func (h *HealthChecker) Ping(ctx context.Context) error {
	if err := h.db.Ping(ctx); err != nil {
		return err
	}
	if h.schemaReady.Load() {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.schemaReady.Load() {
		return nil
	}
	if err := h.verifySchema(ctx); err != nil {
		return err
	}
	h.schemaReady.Store(true)
	return nil
}

func (h *HealthChecker) verifySchema(ctx context.Context) error {
	var (
		currentSchema      string
		missingTables      []string
		unprivilegedTables []string
		missingColumns     []string
		hasPartialEmailIdx bool
	)
	if err := h.db.QueryRow(ctx, schemaCheckSQL, requiredTables).Scan(
		&currentSchema,
		&missingTables,
		&unprivilegedTables,
		&missingColumns,
		&hasPartialEmailIdx,
	); err != nil {
		return fmt.Errorf("verify database schema: %w", err)
	}
	if currentSchema == "" {
		return fmt.Errorf("database schema is out of date (no accessible schema in search_path)")
	}
	var problems []string
	if len(missingTables) > 0 {
		problems = append(problems, "missing tables: "+strings.Join(missingTables, ", "))
	}
	if len(unprivilegedTables) > 0 {
		problems = append(problems, "insufficient table privileges: "+strings.Join(unprivilegedTables, ", "))
	}
	if len(missingColumns) > 0 {
		problems = append(problems, "missing columns: "+strings.Join(missingColumns, ", "))
	}
	if !slices.Contains(missingTables, "users") && !hasPartialEmailIdx {
		problems = append(problems, "missing partial unique index: users_email_lower_key")
	}
	if len(problems) > 0 {
		return fmt.Errorf("database schema is out of date (%s)", strings.Join(problems, "; "))
	}
	return nil
}

// NewPool は cfg からプロセス全体で唯一の pgx pool を構築する。明示的な
// MaxConns を設定し、すぐには接続しない（pgxpool v5 は遅延して接続する）。
// 接続性は起動時には確認されず、GET /up の health check がリクエストごとに
// Ping で確認する。pool は cmd/api の run で 1 回だけ作成され、server の
// shutdown 後に close される。
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolCfg.MaxConns = cfg.DBMaxConns
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return pool, nil
}
