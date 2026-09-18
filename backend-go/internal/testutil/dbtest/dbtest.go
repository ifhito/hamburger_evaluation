// Package dbtest provides the shared scaffold for DB-backed tests: a
// per-run PostgreSQL database created via TEST_DATABASE_URL, optionally
// migrated with the SQL files in db/migrations, and dropped through
// t.Cleanup. It is test-only support code and must never be imported by
// production packages.
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// New creates a fresh per-run database, applies all up migrations, and
// returns an open connection to it together with its URL. The database is
// dropped and the connection closed via t.Cleanup. It skips the test when
// TEST_DATABASE_URL is not set.
func New(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	conn, testURL := NewEmpty(t)
	ups, _ := LoadMigrations(t)
	Apply(context.Background(), t, conn, ups)
	return conn, testURL
}

// NewEmpty creates a fresh per-run database without applying migrations and
// returns an open connection to it together with its URL. The database is
// created inside the Postgres instance TEST_DATABASE_URL points at (a
// maintenance database whose user may create and drop databases) so tests
// always start from an empty database without touching the development
// database or its volume. It skips the test when TEST_DATABASE_URL is not
// set.
func NewEmpty(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping DB-backed test")
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
	return conn, testURL
}

// LoadMigrations returns the *.up.sql files in ascending order and the
// *.down.sql files in descending order, as absolute paths under
// db/migrations. It fails the test if any file does not match
// <version>_<name>.{up,down}.sql or if any version does not have exactly one
// up and one down file.
func LoadMigrations(t *testing.T) (ups, downs []string) {
	t.Helper()
	dir := migrationsDir(t)
	entries, err := os.ReadDir(dir)
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
			ups = append(ups, filepath.Join(dir, name))
		case "down":
			downsByVersion[version]++
			downs = append(downs, filepath.Join(dir, name))
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

// Apply applies the given migration files in order, failing the test on the
// first error.
func Apply(ctx context.Context, t *testing.T, conn *pgx.Conn, files []string) {
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

// migrationsDir resolves db/migrations relative to this source file so that
// callers do not depend on their own package location.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve dbtest source file path")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "migrations")
}

// testDBName returns a per-run database name. The pid/timestamp suffix keeps
// concurrent or aborted runs from colliding while staying a valid lowercase
// PostgreSQL identifier.
func testDBName() string {
	return fmt.Sprintf("hamburger_evaluation_go_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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

// withDatabase returns rawURL with its database (path) replaced by name.
func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// mustExec executes sql on conn and fails the test on error.
func mustExec(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
