// Package dbtest は DB を使うテスト向けの共通の足場を提供する。
// TEST_DATABASE_URL を通して実行ごとに作成される PostgreSQL の database で、
// 必要に応じて db/migrations の SQL ファイルで migrate され、t.Cleanup を通して
// drop される。これはテスト専用のサポートコードであり、本番のパッケージから
// 決して import してはならない。
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

// New は、実行ごとの新しい database を作成し、すべての up migration を
// 適用して、その database への open 済みの接続を URL とともに返す。
// database の drop と接続の close は t.Cleanup を通して行われる。
// TEST_DATABASE_URL が設定されていないときはテストを skip する。
func New(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	conn, testURL := NewEmpty(t)
	ups, _ := LoadMigrations(t)
	Apply(context.Background(), t, conn, ups)
	return conn, testURL
}

// NewEmpty は、実行ごとの新しい database を migration を適用せずに作成し、
// その database への open 済みの接続を URL とともに返す。database は
// TEST_DATABASE_URL が指す Postgres インスタンスの内部に作成される
// （database を作成・drop できるユーザーの maintenance database）ので、
// 開発用 database やその volume に触れることなく、テストは常に空の database
// から始まる。TEST_DATABASE_URL が設定されていないときはテストを skip する。
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

// LoadMigrations は、*.up.sql ファイルを昇順で、*.down.sql ファイルを降順で、
// db/migrations 配下の絶対パスとして返す。いずれかのファイルが
// <version>_<name>.{up,down}.sql に一致しない場合、またはいずれかの version に
// up と down のファイルがちょうど 1 つずつない場合、テストを失敗させる。
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

// Apply は、与えられた migration ファイルを順に適用し、最初のエラーで
// テストを失敗させる。
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

// migrationsDir は、db/migrations をこのソースファイルからの相対で解決する。
// これにより、呼び出し側は自身のパッケージの位置に依存しない。
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve dbtest source file path")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "migrations")
}

// testDBName は、実行ごとの database 名を返す。pid/timestamp の接尾辞により、
// 並行または中断された実行同士が衝突せず、しかも有効な小文字の PostgreSQL
// 識別子であり続ける。
func testDBName() string {
	return fmt.Sprintf("hamburger_evaluation_go_test_%d_%d", os.Getpid(), time.Now().UnixNano())
}

// parseMigrationName は、migration のファイル名を、数字の version 接頭辞
// （例："000001"）と direction（"up" または "down"）に分割する。名前が
// <digits>_<name>.up.sql / .down.sql に一致しないとき、ok は false になる。
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

// withDatabase は、rawURL の database（path）を name に置き換えたものを返す。
func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// mustExec は conn 上で sql を実行し、エラーのときテストを失敗させる。
func mustExec(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
