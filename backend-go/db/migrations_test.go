package db_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// testDBName returns a per-run database name. The database is created fresh
// (and dropped) inside the compose Postgres instance so tests always start
// from an empty database without touching the development database or its
// volume; the pid/timestamp suffix keeps concurrent or aborted runs from
// colliding while staying a valid lowercase PostgreSQL identifier.
func testDBName() string {
	return fmt.Sprintf("hamburger_evaluation_go_test_%d_%d", os.Getpid(), time.Now().UnixNano())
}

// TestMigrationsAcceptance covers AC1-AC4 of story S2 against a real
// PostgreSQL. It requires TEST_DATABASE_URL to point at a maintenance
// database (e.g. postgres://postgres:password@localhost:5433/postgres) whose
// user may create and drop databases; without it the test skips.
//
// The subtests are order-dependent (up -> negative inserts -> down -> re-up)
// and therefore run sequentially within this single test.
func TestMigrationsAcceptance(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping DB-backed migration tests")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to admin database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	dbName := testDBName()
	mustExec(ctx, t, admin, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName))
	mustExec(ctx, t, admin, "CREATE DATABASE "+dbName)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName))
	})

	testURL, err := withDatabase(adminURL, dbName)
	if err != nil {
		t.Fatalf("build test database URL: %v", err)
	}
	conn, err := pgx.Connect(ctx, testURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	ups, downs := loadMigrations(t)

	// AC1: from an empty DB, all migrations up yield tables, constraints
	// and indexes.
	applyMigrations(ctx, t, conn, ups)
	t.Run("AC1_up_creates_schema", func(t *testing.T) {
		assertSchemaPresent(ctx, t, conn)
	})

	// AC3: rating outside 1..5 is rejected by the CHECK constraint.
	t.Run("AC3_rating_check_violation", func(t *testing.T) {
		var userID, burgerID int64
		if err := conn.QueryRow(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3) RETURNING id",
			"ac3@example.com", "ac3", "digest").Scan(&userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if err := conn.QueryRow(ctx,
			"INSERT INTO burgers (name) VALUES ($1) RETURNING id", "AC3 Burger").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		_, err := conn.Exec(ctx,
			"INSERT INTO reviews (rating, user_id, burger_id) VALUES (6, $1, $2)", userID, burgerID)
		assertPgError(t, err, "23514", "reviews_rating_check")
	})

	// AC4: a second user with the same email is rejected by the UNIQUE
	// constraint.
	t.Run("AC4_email_unique_violation", func(t *testing.T) {
		const email = "ac4@example.com"
		if _, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3)",
			email, "ac4-first", "digest"); err != nil {
			t.Fatalf("insert first user: %v", err)
		}
		_, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3)",
			email, "ac4-second", "digest")
		assertPgError(t, err, "23505", "users_email_key")
	})

	// AC2: all migrations down return to an empty database.
	applyMigrations(ctx, t, conn, downs)
	t.Run("AC2_down_returns_to_empty_schema", func(t *testing.T) {
		var count int
		if err := conn.QueryRow(ctx,
			"SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'i', 'S', 'v', 'm')").Scan(&count); err != nil {
			t.Fatalf("count leftover relations: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected empty public schema after down, found %d relations", count)
		}
	})

	// Re-up after down must succeed (up/down/re-up cycle).
	applyMigrations(ctx, t, conn, ups)
	t.Run("reup_after_down_recreates_schema", func(t *testing.T) {
		assertSchemaPresent(ctx, t, conn)
	})
}

// loadMigrations returns the *.up.sql files in ascending order and the
// *.down.sql files in descending order. It fails the test if any file does
// not match <version>_<name>.{up,down}.sql or if any version does not have
// exactly one up and one down file.
func loadMigrations(t *testing.T) (ups, downs []string) {
	t.Helper()
	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	upsByVersion := map[string]int{}
	downsByVersion := map[string]int{}
	for _, entry := range entries {
		name := entry.Name()
		version, direction, ok := parseMigrationName(name)
		if !ok {
			t.Fatalf("migration %s does not match <version>_<name>.up.sql / .down.sql", name)
		}
		switch direction {
		case "up":
			upsByVersion[version]++
			ups = append(ups, filepath.Join("migrations", name))
		case "down":
			downsByVersion[version]++
			downs = append(downs, filepath.Join("migrations", name))
		}
	}
	if len(ups) == 0 {
		t.Fatal("no up migrations found in migrations dir")
	}
	versions := map[string]bool{}
	for version := range upsByVersion {
		versions[version] = true
	}
	for version := range downsByVersion {
		versions[version] = true
	}
	sortedVersions := make([]string, 0, len(versions))
	for version := range versions {
		sortedVersions = append(sortedVersions, version)
	}
	sort.Strings(sortedVersions)
	for _, version := range sortedVersions {
		if upsByVersion[version] != 1 || downsByVersion[version] != 1 {
			t.Fatalf("version %s: expected exactly one .up.sql and one .down.sql, got %d up and %d down",
				version, upsByVersion[version], downsByVersion[version])
		}
	}
	sort.Strings(ups)
	sort.Sort(sort.Reverse(sort.StringSlice(downs)))
	return ups, downs
}

// parseMigrationName splits a migration file name into its numeric version
// prefix (e.g. "000001") and direction ("up" or "down"). ok is false when the
// name does not match <digits>_<name>.up.sql / .down.sql.
func parseMigrationName(name string) (version, direction string, ok bool) {
	switch {
	case strings.HasSuffix(name, ".up.sql"):
		direction = "up"
	case strings.HasSuffix(name, ".down.sql"):
		direction = "down"
	default:
		return "", "", false
	}
	version, rest, found := strings.Cut(name, "_")
	if !found || version == "" || rest == "" {
		return "", "", false
	}
	for _, r := range version {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return version, direction, true
}

func applyMigrations(ctx context.Context, t *testing.T, conn *pgx.Conn, files []string) {
	t.Helper()
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
}

func assertSchemaPresent(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()

	wantTables := []string{"burger_stats", "burgers", "reviews", "shops", "shops_burgers", "users"}
	gotTables := queryStrings(ctx, t, conn,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name")
	if strings.Join(gotTables, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("tables mismatch:\n got %v\nwant %v", gotTables, wantTables)
	}

	// Column-level schema: table/column/data_type/is_nullable, ordered by
	// table name then column position, exactly as the migrations define them.
	wantColumns := []string{
		"burger_stats/burger_id/bigint/NO",
		"burger_stats/review_count/bigint/NO",
		"burger_stats/average_rating/double precision/NO",
		"burger_stats/weighted_score/double precision/NO",
		"burger_stats/confidence/double precision/NO",
		"burger_stats/calculated_at/timestamp with time zone/NO",
		"burgers/id/bigint/NO",
		"burgers/name/text/NO",
		"burgers/created_at/timestamp with time zone/NO",
		"burgers/updated_at/timestamp with time zone/NO",
		"reviews/id/bigint/NO",
		"reviews/rating/smallint/NO",
		"reviews/comment/text/YES",
		"reviews/user_id/bigint/NO",
		"reviews/burger_id/bigint/NO",
		"reviews/discarded_at/timestamp with time zone/YES",
		"reviews/created_at/timestamp with time zone/NO",
		"reviews/updated_at/timestamp with time zone/NO",
		"shops/id/bigint/NO",
		"shops/name/text/NO",
		"shops/status/smallint/NO",
		"shops/moderation_note/text/YES",
		"shops/creator_id/bigint/YES",
		"shops/created_at/timestamp with time zone/NO",
		"shops/updated_at/timestamp with time zone/NO",
		"shops_burgers/shop_id/bigint/NO",
		"shops_burgers/burger_id/bigint/NO",
		"users/id/bigint/NO",
		"users/email/text/NO",
		"users/username/text/NO",
		"users/password_digest/text/NO",
		"users/admin/boolean/NO",
		"users/discarded_at/timestamp with time zone/YES",
		"users/created_at/timestamp with time zone/NO",
		"users/updated_at/timestamp with time zone/NO",
	}
	gotColumns := queryStrings(ctx, t, conn,
		"SELECT table_name || '/' || column_name || '/' || data_type || '/' || is_nullable FROM information_schema.columns WHERE table_schema = 'public' ORDER BY table_name, ordinal_position")
	if strings.Join(gotColumns, "\n") != strings.Join(wantColumns, "\n") {
		t.Fatalf("columns mismatch:\n got:\n%s\nwant:\n%s",
			strings.Join(gotColumns, "\n"), strings.Join(wantColumns, "\n"))
	}

	// Key constraints: contype is p=primary key, u=unique, c=check,
	// f=foreign key.
	constraints := map[string]bool{}
	rows, err := conn.Query(ctx,
		"SELECT conrelid::regclass::text, conname, contype::text FROM pg_constraint WHERE connamespace = 'public'::regnamespace")
	if err != nil {
		t.Fatalf("query pg_constraint: %v", err)
	}
	for rows.Next() {
		var table, name, ctype string
		if err := rows.Scan(&table, &name, &ctype); err != nil {
			t.Fatalf("scan pg_constraint row: %v", err)
		}
		constraints[table+"/"+name+"/"+ctype] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pg_constraint rows: %v", err)
	}
	wantConstraints := []string{
		"users/users_pkey/p",
		"users/users_email_key/u",
		"shops/shops_pkey/p",
		"shops/shops_status_check/c",
		"shops/shops_creator_id_fkey/f",
		"burgers/burgers_pkey/p",
		"shops_burgers/shops_burgers_shop_id_burger_id_key/u",
		"shops_burgers/shops_burgers_shop_id_fkey/f",
		"shops_burgers/shops_burgers_burger_id_fkey/f",
		"reviews/reviews_pkey/p",
		"reviews/reviews_rating_check/c",
		"reviews/reviews_user_id_fkey/f",
		"reviews/reviews_burger_id_fkey/f",
		"burger_stats/burger_stats_burger_id_key/u",
		"burger_stats/burger_stats_burger_id_fkey/f",
	}
	for _, want := range wantConstraints {
		if !constraints[want] {
			t.Errorf("missing constraint %s (have %v)", want, constraints)
		}
	}

	indexes := map[string]bool{}
	for _, name := range queryStrings(ctx, t, conn,
		"SELECT indexname FROM pg_indexes WHERE schemaname = 'public'") {
		indexes[name] = true
	}
	wantIndexes := []string{
		"idx_shops_status",
		"idx_shops_creator_id",
		"idx_shops_burgers_burger_id",
		"idx_reviews_user_id",
		"idx_reviews_burger_id",
	}
	for _, want := range wantIndexes {
		if !indexes[want] {
			t.Errorf("missing index %s (have %v)", want, indexes)
		}
	}
}

func queryStrings(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string) []string {
	t.Helper()
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect rows for %q: %v", sql, err)
	}
	return values
}

func assertPgError(t *testing.T, err error, wantCode, wantConstraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected constraint violation %s (%s), got no error", wantConstraint, wantCode)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != wantCode || pgErr.ConstraintName != wantConstraint {
		t.Fatalf("expected SQLSTATE %s on constraint %s, got SQLSTATE %s on constraint %q: %v",
			wantCode, wantConstraint, pgErr.Code, pgErr.ConstraintName, pgErr)
	}
}

func mustExec(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// withDatabase returns rawURL with its database (path) replaced by name.
func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	u.Path = "/" + name
	return u.String(), nil
}
