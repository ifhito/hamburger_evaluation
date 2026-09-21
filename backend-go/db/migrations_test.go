package db_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestMigrationsAcceptance は、実際の PostgreSQL に対して story S2 の
// AC1-AC4 をカバーする。TEST_DATABASE_URL が、database を作成・drop できる
// ユーザーの maintenance database
// （例：postgres://postgres:password@localhost:5433/postgres）を指している
// 必要がある。設定されていない場合、テストは skip される（dbtest.NewEmpty
// の内部で）。
//
// subtest は実行順序に依存する（up -> negative insert -> down -> re-up）ため、
// この 1 つのテストの中で順次実行される。
func TestMigrationsAcceptance(t *testing.T) {
	ctx := context.Background()
	conn, _ := dbtest.NewEmpty(t)
	ups, downs := dbtest.LoadMigrations(t)

	// AC1：空の DB から、すべての migration を up すると table、制約、
	// index が得られる。
	dbtest.Apply(ctx, t, conn, ups)
	t.Run("AC1 up すると schema が作られる", func(t *testing.T) {
		assertSchemaPresent(ctx, t, conn)
	})

	// AC3：1..5 の範囲外の rating は CHECK 制約によって拒否される。
	t.Run("AC3 範囲外の rating は CHECK 制約違反になる", func(t *testing.T) {
		var userID string
		var burgerID int64
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

	// S27：users.id は DB が uuid（v4）を生成し、それを参照する reviews.user_id・
	// shops.creator_id の外部キーが uuid で効く。
	t.Run("S27 users の id は uuid で生成され、外部キーが uuid で効く", func(t *testing.T) {
		var userID string
		if err := conn.QueryRow(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3) RETURNING id::text",
			"s27@example.com", "s27", "digest").Scan(&userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(userID) {
			t.Fatalf("users.id = %q, want a lowercase v4 uuid", userID)
		}
		if _, err := conn.Exec(ctx,
			"INSERT INTO shops (name, status, creator_id) VALUES ('S27 Shop', 0, $1)", userID); err != nil {
			t.Fatalf("insert shop with an existing creator: %v", err)
		}
		const missing = "00000000-0000-4000-8000-000000000000"
		_, err := conn.Exec(ctx,
			"INSERT INTO shops (name, status, creator_id) VALUES ('S27 Orphan', 0, $1)", missing)
		assertPgError(t, err, "23503", "shops_creator_id_fkey")
		var burgerID int64
		if err := conn.QueryRow(ctx,
			"INSERT INTO burgers (name) VALUES ('S27 Burger') RETURNING id").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		_, err = conn.Exec(ctx,
			"INSERT INTO reviews (rating, user_id, burger_id) VALUES (3, $1, $2)", missing, burgerID)
		assertPgError(t, err, "23503", "reviews_user_id_fkey")
	})

	// AC4：同じ email を持つ 2 人目のユーザーは UNIQUE 制約によって
	// 拒否される。
	t.Run("AC4 同じ email は UNIQUE 制約違反になる", func(t *testing.T) {
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

	// S21 AC7：上限ちょうどは入り、1 文字超えると CHECK 制約違反になる。
	// 数え方はコードポイント数（日本語・絵文字も 1 文字）。
	t.Run("S21 AC7 文字数の上限を超える値は CHECK 制約違反になる", func(t *testing.T) {
		assertTextLimits(ctx, t, conn)
	})

	// S21 AC9：DB の CHECK の上限の値が、domain の定数と食い違っていない。
	t.Run("S21 AC9 CHECK の上限が domain の定数と一致する", func(t *testing.T) {
		got := checkLimits(ctx, t, conn)
		want := map[string]int{
			"reviews_comment_max_length":       domain.MaxCommentChars,
			"burgers_name_max_length":          domain.MaxBurgerNameChars,
			"shops_name_max_length":            domain.MaxShopNameChars,
			"shops_moderation_note_max_length": domain.MaxModerationNoteChars,
			"users_username_max_length":        domain.MaxUsernameChars,
			"users_bio_max_length":             domain.MaxBioChars,
			"users_email_max_length":           domain.MaxEmailChars,
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("CHECK の上限が domain の定数と違う:\n got %v\nwant %v", got, want)
		}
	})

	// AC2：すべての migration を down すると空の database に戻る。
	dbtest.Apply(ctx, t, conn, downs)
	t.Run("AC2 down すると空の schema に戻る", func(t *testing.T) {
		var count int
		if err := conn.QueryRow(ctx,
			"SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'i', 'S', 'v', 'm')").Scan(&count); err != nil {
			t.Fatalf("count leftover relations: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected empty public schema after down, found %d relations", count)
		}
	})

	// down の後の re-up は成功しなければならない（up/down/re-up のサイクル）。
	dbtest.Apply(ctx, t, conn, ups)
	t.Run("down 後に再度 up すると schema が作られる", func(t *testing.T) {
		assertSchemaPresent(ctx, t, conn)
	})
}

func assertSchemaPresent(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()

	wantTables := []string{"burger_stats", "burgers", "reviews", "shops", "shops_burgers", "users"}
	gotTables := queryStrings(ctx, t, conn,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name")
	if strings.Join(gotTables, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("tables mismatch:\n got %v\nwant %v", gotTables, wantTables)
	}

	// カラム単位の schema：table/column/data_type/is_nullable を、table 名、
	// 次にカラムの位置の順に並べたもので、migration が定義するとおりである。
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
		"reviews/user_id/uuid/NO",
		"reviews/burger_id/bigint/NO",
		"reviews/discarded_at/timestamp with time zone/YES",
		"reviews/created_at/timestamp with time zone/NO",
		"reviews/updated_at/timestamp with time zone/NO",
		// photo_key は 000007（S10）で追加されたので、ordinal position では
		// 最後に来る。
		"reviews/photo_key/text/YES",
		"shops/id/bigint/NO",
		"shops/name/text/NO",
		"shops/status/smallint/NO",
		"shops/moderation_note/text/YES",
		"shops/creator_id/uuid/YES",
		"shops/created_at/timestamp with time zone/NO",
		"shops/updated_at/timestamp with time zone/NO",
		"shops_burgers/shop_id/bigint/NO",
		"shops_burgers/burger_id/bigint/NO",
		"users/id/uuid/NO",
		"users/email/text/NO",
		"users/username/text/NO",
		"users/bio/text/NO",
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

	// 主要な制約：contype は p=primary key、u=unique、c=check、f=foreign key
	// である。
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
		"reviews/reviews_comment_max_length/c",
		"burgers/burgers_name_max_length/c",
		"shops/shops_name_max_length/c",
		"shops/shops_moderation_note_max_length/c",
		"users/users_username_max_length/c",
		"users/users_bio_max_length/c",
		"users/users_email_max_length/c",
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

// textLimitCase は、1 つの列の上限の検証に使う。insert は、値を $1 に受け取り、
// ほかの必須の列を埋めた INSERT 文である。
type textLimitCase struct {
	name       string
	limit      int
	constraint string
	insert     string
}

var textLimitCases = []textLimitCase{
	{"reviews.comment", domain.MaxCommentChars, "reviews_comment_max_length",
		"INSERT INTO reviews (rating, comment, user_id, burger_id) VALUES (3, $1, (SELECT id FROM users ORDER BY created_at LIMIT 1), (SELECT min(id) FROM burgers))"},
	{"burgers.name", domain.MaxBurgerNameChars, "burgers_name_max_length",
		"INSERT INTO burgers (name) VALUES ($1)"},
	{"shops.name", domain.MaxShopNameChars, "shops_name_max_length",
		"INSERT INTO shops (name, status) VALUES ($1, 0)"},
	{"shops.moderation_note", domain.MaxModerationNoteChars, "shops_moderation_note_max_length",
		"INSERT INTO shops (name, status, moderation_note) VALUES ('note-shop', 2, $1)"},
	{"users.username", domain.MaxUsernameChars, "users_username_max_length",
		"INSERT INTO users (email, username, password_digest) VALUES ('u' || md5(random()::text) || '@example.com', $1, 'digest')"},
	{"users.bio", domain.MaxBioChars, "users_bio_max_length",
		"INSERT INTO users (email, username, password_digest, bio) VALUES ('b' || md5(random()::text) || '@example.com', 'bio-user', 'digest', $1)"},
	{"users.email", domain.MaxEmailChars, "users_email_max_length",
		"INSERT INTO users (email, username, password_digest) VALUES ($1, 'limit-user', 'digest')"},
}

// assertTextLimits は、各列で「上限ちょうど（マルチバイトを含む）は入る」「1 文字超えると
// 制約違反」を確かめる。reviews の insert が参照する user と burger は、事前に用意する。
func assertTextLimits(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(ctx, "INSERT INTO users (email, username, password_digest) VALUES ('seed-limit@example.com', 'seed', 'digest')"); err != nil {
		t.Fatalf("insert seed user: %v", err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO burgers (name) VALUES ('seed burger')"); err != nil {
		t.Fatalf("insert seed burger: %v", err)
	}
	for _, tc := range textLimitCases {
		// users.email は unique なので、値そのものを変えて入れる。上限ちょうどの値は日本語 1 文字
		// (3 バイト)を含めて、バイト数ではなく文字数で数えられることも確かめる。
		exact := strings.Repeat("あ", tc.limit-1) + "a"
		over := strings.Repeat("あ", tc.limit) + "a"
		if tc.name == "users.email" {
			exact = strings.Repeat("a", tc.limit-len("@example.com")) + "@example.com"
			over = "b" + exact
		}
		if _, err := conn.Exec(ctx, tc.insert, exact); err != nil {
			t.Errorf("%s: 上限ちょうど（%d 文字）が入らない: %v", tc.name, tc.limit, err)
		}
		_, err := conn.Exec(ctx, tc.insert, over)
		assertPgError(t, err, "23514", tc.constraint)
	}
}

var checkLimitPattern = regexp.MustCompile(`char_length\(.*\) <= (\d+)`)

// checkLimits は、*_max_length の CHECK 制約の名前と、その上限の値を返す。
func checkLimits(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]int {
	t.Helper()
	rows, err := conn.Query(ctx,
		"SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE connamespace = 'public'::regnamespace AND contype = 'c' AND conname LIKE '%\\_max\\_length'")
	if err != nil {
		t.Fatalf("query max_length constraints: %v", err)
	}
	defer rows.Close()
	limits := map[string]int{}
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		m := checkLimitPattern.FindStringSubmatch(def)
		if m == nil {
			t.Fatalf("constraint %s の定義から上限を読み取れない: %s", name, def)
		}
		n, _ := strconv.Atoi(m[1])
		limits[name] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate constraints: %v", err)
	}
	return limits
}
