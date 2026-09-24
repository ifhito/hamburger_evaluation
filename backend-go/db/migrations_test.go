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

// TestMigrationsAcceptance は、実際の PostgreSQL に対して、マイグレーションの適用・
// 制約・取り消し(down)が期待どおりに動くことを確かめる。TEST_DATABASE_URL が、database を作成・drop できる
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

	// 空の DB から、すべての migration を up すると table、制約、
	// index が得られる。
	dbtest.Apply(ctx, t, conn, ups)
	t.Run("空の DB にすべてのマイグレーションを up すると、表・制約・インデックスが作られる", func(t *testing.T) {
		assertSchemaPresent(ctx, t, conn)
	})

	// 1..5 の範囲外の rating は CHECK 制約によって拒否される。
	t.Run("評価が 1〜5 の範囲外(6)のレビューを入れると、CHECK 制約違反になる", func(t *testing.T) {
		var userID string
		var burgerID string
		if err := conn.QueryRow(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3) RETURNING id",
			"ac3@example.com", "ac3", "digest").Scan(&userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if err := conn.QueryRow(ctx,
			"INSERT INTO burgers (name) VALUES ($1) RETURNING id", "Rating Check Burger").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		_, err := conn.Exec(ctx,
			"INSERT INTO reviews (rating, user_id, burger_id) VALUES (6, $1, $2)", userID, burgerID)
		assertPgError(t, err, "23514", "reviews_rating_check")
	})

	// users.id は DB が uuid（v4）を生成し、それを参照する reviews.user_id・
	// shops.creator_id の外部キーが uuid で効く。
	t.Run("ユーザーの id は uuid で生成され、存在しないユーザーの uuid を指す外部キーは拒否される", func(t *testing.T) {
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
			"INSERT INTO shops (name, status, creator_id) VALUES ('FK Check Shop', 0, $1)", userID); err != nil {
			t.Fatalf("insert shop with an existing creator: %v", err)
		}
		const missing = "00000000-0000-4000-8000-000000000000"
		_, err := conn.Exec(ctx,
			"INSERT INTO shops (name, status, creator_id) VALUES ('FK Orphan Shop', 0, $1)", missing)
		assertPgError(t, err, "23503", "shops_creator_id_fkey")
		var burgerID string
		if err := conn.QueryRow(ctx,
			"INSERT INTO burgers (name) VALUES ('FK Check Burger') RETURNING id").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		_, err = conn.Exec(ctx,
			"INSERT INTO reviews (rating, user_id, burger_id) VALUES (3, $1, $2)", missing, burgerID)
		assertPgError(t, err, "23503", "reviews_user_id_fkey")
	})

	// shops と burgers の id は DB が UUID（v4）で自動生成する。それを参照する外部キー
	// （shops_burgers・reviews・burger_stats）は、存在しない UUID を拒否する。
	t.Run("ショップとバーガーの id は UUID で自動生成され、存在しない id を参照する行は外部キーで拒否される", func(t *testing.T) {
		v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
		var userID, shopID, burgerID string
		if err := conn.QueryRow(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('fk-check@example.com', 'fk-check', 'digest') RETURNING id::text").Scan(&userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if err := conn.QueryRow(ctx, "INSERT INTO shops (name, status) VALUES ('外部キーの確認用ショップ', 1) RETURNING id::text").Scan(&shopID); err != nil {
			t.Fatalf("insert shop: %v", err)
		}
		if err := conn.QueryRow(ctx, "INSERT INTO burgers (name) VALUES ('外部キーの確認用バーガー') RETURNING id::text").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		for name, id := range map[string]string{"shops.id": shopID, "burgers.id": burgerID} {
			if !v4.MatchString(id) {
				t.Errorf("%s = %q, want a lowercase v4 uuid", name, id)
			}
		}
		if _, err := conn.Exec(ctx, "INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)", shopID, burgerID); err != nil {
			t.Fatalf("link existing shop and burger: %v", err)
		}
		const missing = "00000000-0000-4000-8000-000000000000"
		for _, tt := range []struct {
			name       string
			sql        string
			args       []any
			constraint string
		}{
			{"存在しないショップにバーガーを紐づけようとすると外部キー違反になる", "INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)", []any{missing, burgerID}, "shops_burgers_shop_id_fkey"},
			{"存在しないバーガーをショップに紐づけようとすると外部キー違反になる", "INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)", []any{shopID, missing}, "shops_burgers_burger_id_fkey"},
			{"存在しないバーガーへのレビューを作ろうとすると外部キー違反になる", "INSERT INTO reviews (rating, user_id, burger_id) VALUES (3, $1, $2)", []any{userID, missing}, "reviews_burger_id_fkey"},
			{"存在しないバーガーの統計を作ろうとすると外部キー違反になる", "INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at) VALUES ($1, 0, 0, 0, 0, now())", []any{missing}, "burger_stats_burger_id_fkey"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := conn.Exec(ctx, tt.sql, tt.args...)
				assertPgError(t, err, "23503", tt.constraint)
			})
		}
	})

	// レビューの id も DB が UUID（v4）で自動生成する。これで、すべての表の主キーと、
	// 名前が `_id` で終わる参照の列が UUID になり、連番の id は残っていない。
	t.Run("レビューの id は UUID で自動生成され、すべての表の id と参照の列が UUID になっている", func(t *testing.T) {
		var userID, burgerID, reviewID string
		if err := conn.QueryRow(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('review-id@example.com', 'review-id', 'digest') RETURNING id::text").Scan(&userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if err := conn.QueryRow(ctx, "INSERT INTO burgers (name) VALUES ('レビューの id の確認用バーガー') RETURNING id::text").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		if err := conn.QueryRow(ctx,
			"INSERT INTO reviews (rating, user_id, burger_id) VALUES (3, $1, $2) RETURNING id::text", userID, burgerID).Scan(&reviewID); err != nil {
			t.Fatalf("insert review: %v", err)
		}
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(reviewID) {
			t.Errorf("reviews.id = %q, want a lowercase v4 uuid", reviewID)
		}
		// provider_user_id は、外部のサービス(Google など)が付ける、その利用者の ID(文字列)で、このアプリが採番する ID ではない。
		// OAuth の client_id は、このアプリが採番する ID ではなく、アプリ(クライアント)が自分を名乗る
		// 識別子(自分の説明を公開している URL など)なので、uuid にしない。
		rows, err := conn.Query(ctx, `SELECT table_name || '.' || column_name || ':' || data_type
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name <> 'schema_migrations'
			  AND (column_name = 'id' OR column_name LIKE '%\_id') AND column_name NOT IN ('client_id', 'provider_user_id') AND data_type <> 'uuid'
			ORDER BY 1`)
		if err != nil {
			t.Fatalf("query id columns: %v", err)
		}
		defer rows.Close()
		var notUUID []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatalf("scan: %v", err)
			}
			notUUID = append(notUUID, c)
		}
		if len(notUUID) != 0 {
			t.Errorf("UUID ではない id の列がある: %v", notUUID)
		}
	})

	// 同じ email を持つ 2 人目のユーザーは UNIQUE 制約によって
	// 拒否される。
	t.Run("同じメールアドレスの 2 人目のユーザーを入れると、UNIQUE 制約違反になる", func(t *testing.T) {
		const email = "ac4@example.com"
		if _, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3)",
			email, "ac4-first", "digest"); err != nil {
			t.Fatalf("insert first user: %v", err)
		}
		_, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3)",
			email, "ac4-second", "digest")
		// まったく同じ文字列は、UNIQUE(email)と lower(email)の一意の索引の、どちらにも違反する(先に検出された名前で報告される)。
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" || (pgErr.ConstraintName != "users_email_key" && pgErr.ConstraintName != "users_email_lower_key") {
			t.Fatalf("expected a unique violation on the email, got %v", err)
		}
	})

	// メールの一意性は、大文字小文字を区別せずに、DB が保証する(事前の確認だけに頼らない)。退会済みも対象。
	t.Run("大文字小文字だけが違うメールアドレスの 2 人目のユーザーを入れると、UNIQUE 制約違反になる", func(t *testing.T) {
		if _, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('Case.Mixed@Example.com', 'case-first', 'digest')"); err != nil {
			t.Fatalf("insert first user: %v", err)
		}
		_, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('case.mixed@example.com', 'case-second', 'digest')")
		assertPgError(t, err, "23505", "users_email_lower_key")
	})

	// メール確認待ちの登録(signup_verifications)の制約。トークンのハッシュは一意、email は大文字小文字を
	// 区別せず一意で、必須の列は NOT NULL である。users のスキーマは変わらない。
	t.Run("メール確認待ちの登録は、トークンのハッシュとメールアドレス(大文字小文字を区別しない)が一意で、必須の列は NOT NULL である", func(t *testing.T) {
		const insert = "INSERT INTO signup_verifications (email, username, password_digest, token_hash, expires_at) VALUES ($1, 'u', 'digest', $2, now() + interval '1 day')"
		if _, err := conn.Exec(ctx, insert, "Pending@example.com", "hash-1"); err != nil {
			t.Fatalf("insert first verification: %v", err)
		}
		_, err := conn.Exec(ctx, insert, "other@example.com", "hash-1")
		assertPgError(t, err, "23505", "signup_verifications_token_hash_key")
		_, err = conn.Exec(ctx, insert, "PENDING@example.com", "hash-2")
		assertPgError(t, err, "23505", "idx_signup_verifications_email_lower")
		_, err = conn.Exec(ctx,
			"INSERT INTO signup_verifications (email, username, password_digest, token_hash) VALUES ('n@example.com', 'u', 'digest', 'hash-3')")
		assertPgError(t, err, "23502", "")
	})

	// 上限ちょうどは入り、1 文字超えると CHECK 制約違反になる。
	// 数え方はコードポイント数（日本語・絵文字も 1 文字）。
	t.Run("文字数の上限ちょうどは入り、1 文字超えると CHECK 制約違反になる", func(t *testing.T) {
		assertTextLimits(ctx, t, conn)
	})

	// DB の CHECK の上限の値が、domain の定数と食い違っていない。
	t.Run("DB の CHECK 制約の上限が、domain の定数と一致する", func(t *testing.T) {
		got := checkLimits(ctx, t, conn)
		want := map[string]int{
			"reviews_comment_max_length":       domain.MaxCommentChars,
			"burgers_name_max_length":          domain.MaxBurgerNameChars,
			"shops_name_max_length":            domain.MaxShopNameChars,
			"shops_moderation_note_max_length": domain.MaxModerationNoteChars,
			"shops_map_url_max_length":         domain.MaxMapURLChars,
			"users_username_max_length":        domain.MaxUsernameChars,
			"users_bio_max_length":             domain.MaxBioChars,
			"users_email_max_length":           domain.MaxEmailChars,
			"user_identities_email_max_length": domain.MaxEmailChars,

			"burger_stats_recalc_requests_last_error_max_length": domain.MaxRecalcFailureReasonChars,
			"shop_stats_recalc_requests_last_error_max_length":   domain.MaxRecalcFailureReasonChars,

			"oauth_grants_client_id_max_length":   domain.MaxOAuthURILength,
			"oauth_grants_client_name_max_length": domain.MaxOAuthClientNameLength,
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("CHECK の上限が domain の定数と違う:\n got %v\nwant %v", got, want)
		}
	})

	// OAuth の許可の記録とトークンの記録の制約。許可は利用者とアプリの組ごとに 1 つで、範囲が空・
	// 長すぎる表示名は入らない。利用者を消すと許可が、許可を消すとトークンの記録が、連鎖して消える。
	t.Run("OAuth の許可とトークンの記録は、制約を守り、利用者・許可の削除に連鎖して消える", func(t *testing.T) {
		var userID, grantID string
		if err := conn.QueryRow(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('oauth@example.com', 'oauth', 'digest') RETURNING id::text").Scan(&userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		const insertGrant = "INSERT INTO oauth_grants (user_id, client_id, client_name, scopes) VALUES ($1, $2, $3, $4) RETURNING id::text"
		if err := conn.QueryRow(ctx, insertGrant, userID, "app", "アプリ", []string{"hamburger:read"}).Scan(&grantID); err != nil {
			t.Fatalf("insert grant: %v", err)
		}
		_, err := conn.Exec(ctx, insertGrant, userID, "app", "別名", []string{"hamburger:read"})
		assertPgError(t, err, "23505", "oauth_grants_user_client_key")
		_, err = conn.Exec(ctx, insertGrant, userID, "app2", "アプリ", []string{})
		assertPgError(t, err, "23514", "oauth_grants_scopes_not_empty")
		_, err = conn.Exec(ctx, insertGrant, userID, "app2", strings.Repeat("あ", domain.MaxOAuthClientNameLength+1), []string{"hamburger:read"})
		assertPgError(t, err, "23514", "oauth_grants_client_name_max_length")
		_, err = conn.Exec(ctx, insertGrant, userID, "app2", "", []string{"hamburger:read"})
		assertPgError(t, err, "23514", "oauth_grants_client_name_not_empty")
		_, err = conn.Exec(ctx, insertGrant, userID, strings.Repeat("a", domain.MaxOAuthURILength+1), "アプリ", []string{"hamburger:read"})
		assertPgError(t, err, "23514", "oauth_grants_client_id_max_length")

		const insertSession = "INSERT INTO oauth_token_sessions (kind, signature, request_id, grant_id, user_id, client_id, request, expires_at) VALUES ($1, $2, gen_random_uuid(), $3, $4, 'app', '{}', now())"
		_, err = conn.Exec(ctx, insertSession, "password", "s0", grantID, userID)
		assertPgError(t, err, "23514", "oauth_token_sessions_kind_check")
		if _, err := conn.Exec(ctx, insertSession, "access_token", "s1", grantID, userID); err != nil {
			t.Fatalf("insert token session: %v", err)
		}
		_, err = conn.Exec(ctx, insertSession, "access_token", "s1", grantID, userID)
		assertPgError(t, err, "23505", "oauth_token_sessions_kind_signature_key")
		if _, err := conn.Exec(ctx, insertSession, "refresh_token", "s1", grantID, userID); err != nil {
			t.Errorf("同じ署名でも、種類が違えば入るはず: %v", err)
		}
		_, err = conn.Exec(ctx, insertSession, "access_token", "s2", "00000000-0000-4000-8000-000000000000", userID)
		assertPgError(t, err, "23503", "oauth_token_sessions_grant_id_fkey")

		if _, err := conn.Exec(ctx, "DELETE FROM oauth_grants WHERE id = $1", grantID); err != nil {
			t.Fatalf("delete grant: %v", err)
		}
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM oauth_token_sessions").Scan(&n); err != nil || n != 0 {
			t.Errorf("許可を消したあとのトークンの記録 = %d (%v), want 0", n, err)
		}
		if err := conn.QueryRow(ctx, insertGrant, userID, "app", "アプリ", []string{"hamburger:read"}).Scan(&grantID); err != nil {
			t.Fatalf("re-insert grant: %v", err)
		}
		if _, err := conn.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Fatalf("delete user: %v", err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM oauth_grants").Scan(&n); err != nil || n != 0 {
			t.Errorf("利用者を消したあとの許可の記録 = %d (%v), want 0", n, err)
		}
	})

	// 再計算の依頼は、バーガーごとに 1 行だけで、存在しないバーガーと負の失敗回数は入らない。
	// version は、行を消して作り直しても、前の値に戻らない(戻ると、古い再計算が、新しく入った
	// 依頼を、同じ番号だと思って消してしまう)。
	t.Run("再計算の依頼は、バーガーごとに 1 行で、消して作り直しても version が戻らない", func(t *testing.T) {
		var burgerID string
		if err := conn.QueryRow(ctx, "INSERT INTO burgers (name) VALUES ('依頼の確認用バーガー') RETURNING id::text").Scan(&burgerID); err != nil {
			t.Fatalf("insert burger: %v", err)
		}
		insertVersion := func() int64 {
			t.Helper()
			var v int64
			if err := conn.QueryRow(ctx, "INSERT INTO burger_stats_recalc_requests (burger_id) VALUES ($1) RETURNING version", burgerID).Scan(&v); err != nil {
				t.Fatalf("insert burger_stats_recalc_requests: %v", err)
			}
			return v
		}
		first := insertVersion()
		_, err := conn.Exec(ctx, "INSERT INTO burger_stats_recalc_requests (burger_id) VALUES ($1)", burgerID)
		assertPgError(t, err, "23505", "burger_stats_recalc_requests_pkey")

		if _, err := conn.Exec(ctx, "DELETE FROM burger_stats_recalc_requests WHERE burger_id = $1", burgerID); err != nil {
			t.Fatalf("delete burger_stats_recalc_requests: %v", err)
		}
		if second := insertVersion(); second <= first {
			t.Errorf("作り直した依頼の version = %d, want > %d", second, first)
		}

		_, err = conn.Exec(ctx, "UPDATE burger_stats_recalc_requests SET attempts = -1 WHERE burger_id = $1", burgerID)
		assertPgError(t, err, "23514", "burger_stats_recalc_requests_attempts_check")
		_, err = conn.Exec(ctx, "INSERT INTO burger_stats_recalc_requests (burger_id) VALUES ('00000000-0000-4000-8000-000000000000')")
		assertPgError(t, err, "23503", "burger_stats_recalc_requests_burger_id_fkey")
	})

	// ショップの集計と、その再計算の依頼(000015・000016)。集計はショップごとに 1 行で、件数は負にならず、平均は
	// 1〜5 で、レビューがあるときだけ入る(件数 0 なら NULL)。ショップを消すと、集計と依頼が連鎖して消える。
	// 依頼の version は、バーガーの依頼と同じく、消して作り直しても戻らない。
	t.Run("ショップの集計と依頼は、制約を守り、ショップの削除に連鎖して消え、依頼の version は戻らない", func(t *testing.T) {
		var shopID string
		if err := conn.QueryRow(ctx, "INSERT INTO shops (name, status) VALUES ('集計の確認用ショップ', 1) RETURNING id::text").Scan(&shopID); err != nil {
			t.Fatalf("insert shop: %v", err)
		}
		insertStat := "INSERT INTO shop_stats (shop_id, review_count, average_rating, calculated_at) VALUES ($1, $2, $3, now())"
		if _, err := conn.Exec(ctx, insertStat, shopID, 2, 4.5); err != nil {
			t.Fatalf("insert shop_stats: %v", err)
		}
		_, err := conn.Exec(ctx, insertStat, shopID, 2, 4.5)
		assertPgError(t, err, "23505", "shop_stats_pkey")
		for _, tt := range []struct {
			name       string
			sql        string
			args       []any
			code       string
			constraint string
		}{
			{"件数が負の集計は入らない", "UPDATE shop_stats SET review_count = -1, average_rating = NULL WHERE shop_id = $1", []any{shopID}, "23514", "shop_stats_review_count_check"},
			{"平均が 1 未満の集計は入らない", "UPDATE shop_stats SET average_rating = 0.9 WHERE shop_id = $1", []any{shopID}, "23514", "shop_stats_average_rating_check"},
			{"平均が 5 を超える集計は入らない", "UPDATE shop_stats SET average_rating = 5.1 WHERE shop_id = $1", []any{shopID}, "23514", "shop_stats_average_rating_check"},
			{"レビューがあるのに平均がない集計は入らない", "UPDATE shop_stats SET average_rating = NULL WHERE shop_id = $1", []any{shopID}, "23514", "shop_stats_average_rating_presence_check"},
			{"レビューがないのに平均がある集計は入らない", "UPDATE shop_stats SET review_count = 0 WHERE shop_id = $1", []any{shopID}, "23514", "shop_stats_average_rating_presence_check"},
			{"存在しないショップの集計は入らない", insertStat, []any{"00000000-0000-4000-8000-000000000000", 1, 3.0}, "23503", "shop_stats_shop_id_fkey"},
			{"存在しないショップの依頼は入らない", "INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)", []any{"00000000-0000-4000-8000-000000000000"}, "23503", "shop_stats_recalc_requests_shop_id_fkey"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := conn.Exec(ctx, tt.sql, tt.args...)
				assertPgError(t, err, tt.code, tt.constraint)
			})
		}
		if _, err := conn.Exec(ctx, "INSERT INTO shop_stats (shop_id, review_count, calculated_at) VALUES ($1, 0, now()) ON CONFLICT (shop_id) DO UPDATE SET review_count = 0, average_rating = NULL", shopID); err != nil {
			t.Errorf("レビューなし(件数 0・平均 NULL)の集計を保存できない: %v", err)
		}

		insertVersion := func() int64 {
			t.Helper()
			var v int64
			if err := conn.QueryRow(ctx, "INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1) RETURNING version", shopID).Scan(&v); err != nil {
				t.Fatalf("insert shop_stats_recalc_requests: %v", err)
			}
			return v
		}
		first := insertVersion()
		_, err = conn.Exec(ctx, "INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)", shopID)
		assertPgError(t, err, "23505", "shop_stats_recalc_requests_pkey")
		if _, err := conn.Exec(ctx, "DELETE FROM shop_stats_recalc_requests WHERE shop_id = $1", shopID); err != nil {
			t.Fatalf("delete shop_stats_recalc_requests: %v", err)
		}
		if second := insertVersion(); second <= first {
			t.Errorf("作り直した依頼の version = %d, want > %d", second, first)
		}
		_, err = conn.Exec(ctx, "UPDATE shop_stats_recalc_requests SET attempts = -1 WHERE shop_id = $1", shopID)
		assertPgError(t, err, "23514", "shop_stats_recalc_requests_attempts_check")

		if _, err := conn.Exec(ctx, "DELETE FROM shops WHERE id = $1", shopID); err != nil {
			t.Fatalf("delete shop: %v", err)
		}
		var stats, requests int
		if err := conn.QueryRow(ctx, "SELECT (SELECT count(*) FROM shop_stats WHERE shop_id = $1), (SELECT count(*) FROM shop_stats_recalc_requests WHERE shop_id = $1)", shopID).Scan(&stats, &requests); err != nil {
			t.Fatal(err)
		}
		if stats != 0 || requests != 0 {
			t.Errorf("ショップを消したあとの集計 = %d 行・依頼 = %d 行, want どちらも 0(連鎖して消える)", stats, requests)
		}
	})

	// すべての migration を down すると空の database に戻る。
	dbtest.Apply(ctx, t, conn, downs)
	t.Run("すべてのマイグレーションを down すると、空のスキーマに戻る", func(t *testing.T) {
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

	wantTables := []string{"burger_stats", "burger_stats_recalc_requests", "burgers", "login_handoffs", "mail_deliveries", "oauth_grants", "oauth_token_sessions", "reviews", "shop_stats", "shop_stats_recalc_requests", "shops", "shops_burgers", "signup_verifications", "user_identities", "users"}
	gotTables := queryStrings(ctx, t, conn,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name")
	if strings.Join(gotTables, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("tables mismatch:\n got %v\nwant %v", gotTables, wantTables)
	}

	// カラム単位の schema：table/column/data_type/is_nullable を、table 名、
	// 次にカラムの位置の順に並べたもので、migration が定義するとおりである。
	wantColumns := []string{
		"burger_stats/burger_id/uuid/NO",
		"burger_stats/review_count/bigint/NO",
		"burger_stats/average_rating/double precision/NO",
		"burger_stats/weighted_score/double precision/NO",
		"burger_stats/confidence/double precision/NO",
		"burger_stats/calculated_at/timestamp with time zone/NO",
		// burger_stats_recalc_requests は 000010 で追加された(統計の再計算の依頼)。
		"burger_stats_recalc_requests/burger_id/uuid/NO",
		"burger_stats_recalc_requests/version/bigint/NO",
		"burger_stats_recalc_requests/attempts/integer/NO",
		"burger_stats_recalc_requests/next_attempt_at/timestamp with time zone/YES",
		"burger_stats_recalc_requests/last_error/text/YES",
		"burger_stats_recalc_requests/created_at/timestamp with time zone/NO",
		"burger_stats_recalc_requests/updated_at/timestamp with time zone/NO",
		"burgers/id/uuid/NO",
		"burgers/name/text/NO",
		"burgers/created_at/timestamp with time zone/NO",
		"burgers/updated_at/timestamp with time zone/NO",
		// login_handoffs は 000014 で追加された(外部のサービスでのサインインの結果を、画面へ渡すコード)。
		"login_handoffs/id/uuid/NO",
		"login_handoffs/code_hash/text/NO",
		"login_handoffs/binder_hash/text/NO",
		"login_handoffs/outcome/text/NO",
		"login_handoffs/user_id/uuid/YES",
		"login_handoffs/return_to/text/NO",
		"login_handoffs/expires_at/timestamp with time zone/NO",
		"login_handoffs/created_at/timestamp with time zone/NO",
		// mail_deliveries は 000009 で追加された。
		"mail_deliveries/id/uuid/NO",
		"mail_deliveries/kind/text/NO",
		"mail_deliveries/recipient/text/NO",
		"mail_deliveries/idempotency_key/text/NO",
		"mail_deliveries/status/text/NO",
		"mail_deliveries/failure_kind/text/YES",
		"mail_deliveries/attempts/integer/NO",
		"mail_deliveries/last_error/text/YES",
		"mail_deliveries/created_at/timestamp with time zone/NO",
		"mail_deliveries/sent_at/timestamp with time zone/YES",
		// oauth_grants と oauth_token_sessions は 000011・000012 で追加された(OAuth の認可サーバー)。
		"oauth_grants/id/uuid/NO",
		"oauth_grants/user_id/uuid/NO",
		"oauth_grants/client_id/text/NO",
		"oauth_grants/client_name/text/NO",
		"oauth_grants/scopes/ARRAY/NO",
		"oauth_grants/created_at/timestamp with time zone/NO",
		"oauth_grants/updated_at/timestamp with time zone/NO",
		"oauth_token_sessions/id/uuid/NO",
		"oauth_token_sessions/kind/text/NO",
		"oauth_token_sessions/signature/text/NO",
		"oauth_token_sessions/request_id/uuid/NO",
		"oauth_token_sessions/grant_id/uuid/NO",
		"oauth_token_sessions/user_id/uuid/NO",
		"oauth_token_sessions/client_id/text/NO",
		"oauth_token_sessions/active/boolean/NO",
		"oauth_token_sessions/request/jsonb/NO",
		"oauth_token_sessions/expires_at/timestamp with time zone/NO",
		"oauth_token_sessions/created_at/timestamp with time zone/NO",
		"reviews/id/uuid/NO",
		"reviews/rating/smallint/NO",
		"reviews/comment/text/YES",
		"reviews/user_id/uuid/NO",
		"reviews/burger_id/uuid/NO",
		"reviews/discarded_at/timestamp with time zone/YES",
		"reviews/created_at/timestamp with time zone/NO",
		"reviews/updated_at/timestamp with time zone/NO",
		// photo_key は 000007 で追加されたので、ordinal position では
		// 最後に来る。
		"reviews/photo_key/text/YES",
		// visited_at は 000017 で追加されたので、ordinal position では photo_key の後に来る。
		"reviews/visited_at/date/YES",
		// shop_stats は 000015、shop_stats_recalc_requests は 000016 で追加された(ショップの集計と、その再計算の依頼)。
		"shop_stats/shop_id/uuid/NO",
		"shop_stats/review_count/bigint/NO",
		"shop_stats/average_rating/double precision/YES",
		"shop_stats/photo_key/text/YES",
		"shop_stats/calculated_at/timestamp with time zone/NO",
		"shop_stats_recalc_requests/shop_id/uuid/NO",
		"shop_stats_recalc_requests/version/bigint/NO",
		"shop_stats_recalc_requests/attempts/integer/NO",
		"shop_stats_recalc_requests/next_attempt_at/timestamp with time zone/YES",
		"shop_stats_recalc_requests/last_error/text/YES",
		"shop_stats_recalc_requests/created_at/timestamp with time zone/NO",
		"shop_stats_recalc_requests/updated_at/timestamp with time zone/NO",
		"shops/id/uuid/NO",
		"shops/name/text/NO",
		"shops/status/smallint/NO",
		"shops/moderation_note/text/YES",
		"shops/creator_id/uuid/YES",
		"shops/closed_at/timestamp with time zone/YES",
		"shops/created_at/timestamp with time zone/NO",
		"shops/updated_at/timestamp with time zone/NO",
		// map_url は 000018 で追加されたので、ordinal position では最後に来る。
		"shops/map_url/text/YES",
		"shops_burgers/shop_id/uuid/NO",
		"shops_burgers/burger_id/uuid/NO",
		// signup_verifications は 000008 で追加された。
		"signup_verifications/id/uuid/NO",
		"signup_verifications/email/text/NO",
		"signup_verifications/username/text/NO",
		"signup_verifications/password_digest/text/NO",
		"signup_verifications/token_hash/text/NO",
		"signup_verifications/expires_at/timestamp with time zone/NO",
		"signup_verifications/last_sent_at/timestamp with time zone/NO",
		"signup_verifications/generation/integer/NO",
		"signup_verifications/created_at/timestamp with time zone/NO",
		// user_identities は 000013 で追加された(外部のサービスのアカウントとの結び付き)。
		"user_identities/id/uuid/NO",
		"user_identities/user_id/uuid/NO",
		"user_identities/provider/text/NO",
		"user_identities/provider_user_id/text/NO",
		"user_identities/email/text/NO",
		"user_identities/created_at/timestamp with time zone/NO",
		"users/id/uuid/NO",
		"users/email/text/NO",
		"users/username/text/NO",
		"users/bio/text/NO",
		// パスワードでサインインする方法を持たないアカウントは NULL(外部のサービスだけで作ったもの)。
		"users/password_digest/text/YES",
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
		"shops/shops_map_url_max_length/c",
		"users/users_username_max_length/c",
		"users/users_bio_max_length/c",
		"users/users_email_max_length/c",
		"users/users_password_digest_not_empty/c",
		"reviews/reviews_user_id_fkey/f",
		"reviews/reviews_burger_id_fkey/f",
		"burger_stats/burger_stats_burger_id_key/u",
		"burger_stats/burger_stats_burger_id_fkey/f",
		"signup_verifications/signup_verifications_pkey/p",
		"signup_verifications/signup_verifications_token_hash_key/u",
		"signup_verifications/signup_verifications_generation_check/c",
		"mail_deliveries/mail_deliveries_pkey/p",
		"mail_deliveries/mail_deliveries_idempotency_key_key/u",
		"mail_deliveries/mail_deliveries_kind_check/c",
		"mail_deliveries/mail_deliveries_status_check/c",
		"mail_deliveries/mail_deliveries_sent_at_check/c",
		"burger_stats_recalc_requests/burger_stats_recalc_requests_pkey/p",
		"burger_stats_recalc_requests/burger_stats_recalc_requests_burger_id_fkey/f",
		"burger_stats_recalc_requests/burger_stats_recalc_requests_attempts_check/c",
		"burger_stats_recalc_requests/burger_stats_recalc_requests_last_error_max_length/c",
		"shop_stats/shop_stats_pkey/p",
		"shop_stats/shop_stats_shop_id_fkey/f",
		"shop_stats/shop_stats_review_count_check/c",
		"shop_stats/shop_stats_average_rating_check/c",
		"shop_stats/shop_stats_average_rating_presence_check/c",
		"shop_stats_recalc_requests/shop_stats_recalc_requests_pkey/p",
		"shop_stats_recalc_requests/shop_stats_recalc_requests_shop_id_fkey/f",
		"shop_stats_recalc_requests/shop_stats_recalc_requests_attempts_check/c",
		"shop_stats_recalc_requests/shop_stats_recalc_requests_last_error_max_length/c",
		"oauth_grants/oauth_grants_pkey/p",
		"oauth_grants/oauth_grants_user_id_fkey/f",
		"oauth_grants/oauth_grants_user_client_key/u",
		"oauth_grants/oauth_grants_client_id_max_length/c",
		"oauth_grants/oauth_grants_client_id_not_empty/c",
		"oauth_grants/oauth_grants_client_name_max_length/c",
		"oauth_grants/oauth_grants_client_name_not_empty/c",
		"oauth_grants/oauth_grants_scopes_not_empty/c",
		"oauth_token_sessions/oauth_token_sessions_pkey/p",
		"oauth_token_sessions/oauth_token_sessions_grant_id_fkey/f",
		"oauth_token_sessions/oauth_token_sessions_user_id_fkey/f",
		"oauth_token_sessions/oauth_token_sessions_kind_signature_key/u",
		"oauth_token_sessions/oauth_token_sessions_kind_check/c",
		"oauth_token_sessions/oauth_token_sessions_signature_length/c",
		"user_identities/user_identities_pkey/p",
		"user_identities/user_identities_user_id_fkey/f",
		"user_identities/user_identities_provider_check/c",
		"user_identities/user_identities_provider_user_id_check/c",
		"user_identities/user_identities_email_max_length/c",
		"user_identities/user_identities_provider_user_key/u",
		"user_identities/user_identities_user_provider_key/u",
		"login_handoffs/login_handoffs_pkey/p",
		"login_handoffs/login_handoffs_user_id_fkey/f",
		"login_handoffs/login_handoffs_code_hash_key/u",
		"login_handoffs/login_handoffs_binder_hash_check/c",
		"login_handoffs/login_handoffs_outcome_check/c",
		"login_handoffs/login_handoffs_user_check/c",
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
		"idx_signup_verifications_email_lower",
		"idx_signup_verifications_expires_at",
		"idx_oauth_token_sessions_request_id",
		"idx_oauth_token_sessions_grant_id",
		"idx_oauth_token_sessions_user_id",
		"idx_oauth_token_sessions_expires_at",
		"idx_login_handoffs_expires_at",
		"idx_login_handoffs_user_id",
		"users_email_lower_key",
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
		"INSERT INTO reviews (rating, comment, user_id, burger_id) VALUES (3, $1, (SELECT id FROM users ORDER BY created_at LIMIT 1), (SELECT id FROM burgers ORDER BY created_at LIMIT 1))"},
	{"burgers.name", domain.MaxBurgerNameChars, "burgers_name_max_length",
		"INSERT INTO burgers (name) VALUES ($1)"},
	{"shops.name", domain.MaxShopNameChars, "shops_name_max_length",
		"INSERT INTO shops (name, status) VALUES ($1, 0)"},
	{"shops.moderation_note", domain.MaxModerationNoteChars, "shops_moderation_note_max_length",
		"INSERT INTO shops (name, status, moderation_note) VALUES ('note-shop', 2, $1)"},
	{"shops.map_url", domain.MaxMapURLChars, "shops_map_url_max_length",
		"INSERT INTO shops (name, status, map_url) VALUES ('map-shop', 0, $1)"},
	{"users.username", domain.MaxUsernameChars, "users_username_max_length",
		"INSERT INTO users (email, username, password_digest) VALUES ('u' || md5(random()::text) || '@example.com', $1, 'digest')"},
	{"users.bio", domain.MaxBioChars, "users_bio_max_length",
		"INSERT INTO users (email, username, password_digest, bio) VALUES ('b' || md5(random()::text) || '@example.com', 'bio-user', 'digest', $1)"},
	{"users.email", domain.MaxEmailChars, "users_email_max_length",
		"INSERT INTO users (email, username, password_digest) VALUES ($1, 'limit-user', 'digest')"},
	{"user_identities.email", domain.MaxEmailChars, "user_identities_email_max_length",
		"INSERT INTO user_identities (user_id, provider, provider_user_id, email) VALUES ((SELECT id FROM users ORDER BY created_at LIMIT 1), 'google', 'sub-' || md5(random()::text), $1)"},
	{"burger_stats_recalc_requests.last_error", domain.MaxRecalcFailureReasonChars, "burger_stats_recalc_requests_last_error_max_length",
		"INSERT INTO burger_stats_recalc_requests (burger_id, last_error) VALUES ((SELECT id FROM burgers ORDER BY created_at LIMIT 1), $1)"},
	{"shop_stats_recalc_requests.last_error", domain.MaxRecalcFailureReasonChars, "shop_stats_recalc_requests_last_error_max_length",
		"INSERT INTO shop_stats_recalc_requests (shop_id, last_error) VALUES ((SELECT id FROM shops ORDER BY created_at LIMIT 1), $1)"},
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
