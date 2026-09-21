package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// strPtr は、domain.ProfileChanges のフィールド用に、s へのポインタを返す。
func strPtr(s string) *string { return &s }

// TestUserRepository は、repository を実際の PostgreSQL に対して検証する。
// 共有の dbtest のスキャフォールドを使い、実行ごとのデータベースを compose の
// Postgres インスタンス内に作成して migrate up し、終了後に drop する。
// TEST_DATABASE_URL が、そのユーザーがデータベースを作成・drop できる
// メンテナンス用データベースを指している必要がある。設定がなければ、テストは
// スキップされる（dbtest.New の内部で）。
func TestUserRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)

	repo := repository.NewUserRepository(conn)

	created, err := repo.CreateUser(ctx, domain.CreateUserParams{
		Username:       "alice",
		Email:          "alice@example.com",
		PasswordDigest: "digest-alice",
		Admin:          false,
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if !domain.IsUUID(created.ID) {
		t.Fatalf("CreateUser returned ID %q, want a UUID in the canonical form", created.ID)
	}
	wantUser := domain.User{ID: created.ID, Username: "alice", Email: "alice@example.com", Admin: false}
	if created != wantUser {
		t.Fatalf("CreateUser = %+v, want %+v", created, wantUser)
	}

	t.Run("CreateUser は保存された行に、渡した値と digest を入れる", func(t *testing.T) {
		var username, email, digest string
		var admin bool
		var discardedAt *time.Time
		if err := conn.QueryRow(ctx,
			`SELECT username, email, password_digest, admin, discarded_at FROM users WHERE id = $1`, created.ID,
		).Scan(&username, &email, &digest, &admin, &discardedAt); err != nil {
			t.Fatalf("select created user: %v", err)
		}
		if username != "alice" || email != "alice@example.com" || digest != "digest-alice" || admin || discardedAt != nil {
			t.Errorf("stored = (%q, %q, %q, admin %v, discarded %v), want alice with digest-alice, not admin, kept",
				username, email, digest, admin, discardedAt)
		}
	})

	t.Run("email が重複すると ErrEmailTaken になる", func(t *testing.T) {
		_, err := repo.CreateUser(ctx, domain.CreateUserParams{
			Username:       "alice2",
			Email:          "alice@example.com",
			PasswordDigest: "digest-alice2",
			Admin:          false,
		})
		if !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("CreateUser error = %v, want %v", err, domain.ErrEmailTaken)
		}
	})
}

// TestUserRepositoryManagement は、S8 の user 管理の永続化を検証する。
// 対象は、カラム単位でトランザクションを伴うプロフィール更新、
// 同一トランザクション内での burger_stats の再計算を伴う soft delete、
// そして、discard 済みの user の review をフィード、review の詳細、shop の
// review から読み取り側で除外することである。
func TestUserRepositoryManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewUserRepository(conn)
	reviewRepo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, $3, $4) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", "digest-alice", false)
	bob := dbtest.InsertUserRow(ctx, t, conn, insertUser, "bob@example.com", "bob", "digest-bob", false)
	victim := dbtest.InsertUserRow(ctx, t, conn, insertUser, "victim@example.com", "victim", "digest-victim", false)
	ghost := dbtest.InsertUserRow(ctx, t, conn, insertUser, "ghost@example.com", "ghost", "digest-ghost", false)
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, ghost); err != nil {
		t.Fatalf("discard ghost: %v", err)
	}

	// active な shop 1 つが 2 つの burger を提供している："shared" は victim と
	// alice の両方が review し、"solo" は victim だけが review した。discard 後の
	// stats の再計算と、読み取り経路の見え方は、トランザクションを持つ usecase の
	// UnitOfWork のテスト（adapter/uow）が扱う。
	shop := dbtest.InsertUUIDRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Active One", 1, nil, nil)
	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	shared := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Shared")
	solo := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Solo")
	for _, burgerID := range []string{shared, solo} {
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shop, burgerID); err != nil {
			t.Fatalf("link shop %s burger %s: %v", shop, burgerID, err)
		}
	}
	mustCreateReview(ctx, t, reviewRepo, 2, "meh", victim, shared)
	mustCreateReview(ctx, t, reviewRepo, 4, "good", alice, shared)
	mustCreateReview(ctx, t, reviewRepo, 5, "only mine", victim, solo)

	t.Run("UpdateUserProfile は指定されたフィールドだけを更新する", func(t *testing.T) {
		updated, err := repo.UpdateUserProfile(ctx, bob, domain.ProfileChanges{Username: strPtr("bobby")})
		if err != nil {
			t.Fatalf("UpdateUserProfile returned error: %v", err)
		}
		if want := (domain.User{ID: bob, Username: "bobby", Email: "bob@example.com"}); updated != want {
			t.Fatalf("UpdateUserProfile = %+v, want %+v", updated, want)
		}
		// 更新していないカラムは、ストレージ上でも変更されていない。
		var email, digest string
		if err := conn.QueryRow(ctx, `SELECT email, password_digest FROM users WHERE id = $1`, bob).Scan(&email, &digest); err != nil {
			t.Fatalf("select bob: %v", err)
		}
		if email != "bob@example.com" || digest != "digest-bob" {
			t.Fatalf("stored (email, digest) = (%q, %q), want unchanged", email, digest)
		}
	})

	t.Run("UpdateUserProfile は複数のフィールドを 1 つのトランザクションで更新する", func(t *testing.T) {
		updated, err := repo.UpdateUserProfile(ctx, bob, domain.ProfileChanges{
			Email:          strPtr("bobby@example.com"),
			PasswordDigest: strPtr("digest-bobby"),
		})
		if err != nil {
			t.Fatalf("UpdateUserProfile returned error: %v", err)
		}
		if want := (domain.User{ID: bob, Username: "bobby", Email: "bobby@example.com"}); updated != want {
			t.Fatalf("UpdateUserProfile = %+v, want %+v", updated, want)
		}
		var digest string
		if err := conn.QueryRow(ctx, `SELECT password_digest FROM users WHERE id = $1`, bob).Scan(&digest); err != nil {
			t.Fatalf("select bob: %v", err)
		}
		if digest != "digest-bobby" {
			t.Fatalf("stored digest = %q, want %q", digest, "digest-bobby")
		}
	})

	t.Run("UpdateUserProfile は変更が 0 件なら現在の user を返す", func(t *testing.T) {
		user, err := repo.UpdateUserProfile(ctx, bob, domain.ProfileChanges{})
		if err != nil {
			t.Fatalf("UpdateUserProfile returned error: %v", err)
		}
		if want := (domain.User{ID: bob, Username: "bobby", Email: "bobby@example.com"}); user != want {
			t.Fatalf("UpdateUserProfile = %+v, want unchanged %+v", user, want)
		}
	})

	t.Run("UpdateUserProfile で使用済みの email を指定すると ErrEmailTaken になり rollback される", func(t *testing.T) {
		_, err := repo.UpdateUserProfile(ctx, bob, domain.ProfileChanges{
			Username: strPtr("sneaky"),
			Email:    strPtr("alice@example.com"),
		})
		if !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("UpdateUserProfile error = %v, want %v", err, domain.ErrEmailTaken)
		}
		// 同じ呼び出しの username の変更も、一緒に rollback された。
		var username string
		if err := conn.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, bob).Scan(&username); err != nil {
			t.Fatalf("select bob: %v", err)
		}
		if username != "bobby" {
			t.Fatalf("stored username = %q, want the pre-call %q (rollback)", username, "bobby")
		}
	})

	t.Run("UpdateUserProfile で存在しない id と discard 済みの id は ErrUserNotFound になる", func(t *testing.T) {
		for name, id := range map[string]string{"unknown": uid.N(99999), "discarded": ghost} {
			if _, err := repo.UpdateUserProfile(ctx, id, domain.ProfileChanges{Username: strPtr("x")}); !errors.Is(err, domain.ErrUserNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrUserNotFound)
			}
			if _, err := repo.UpdateUserProfile(ctx, id, domain.ProfileChanges{}); !errors.Is(err, domain.ErrUserNotFound) {
				t.Errorf("%s (zero changes): error = %v, want %v", name, err, domain.ErrUserNotFound)
			}
		}
	})

	t.Run("DiscardUser は user に discard 時刻を刻み、review には触れない", func(t *testing.T) {
		if err := repo.DiscardUser(ctx, victim); err != nil {
			t.Fatalf("DiscardUser returned error: %v", err)
		}
		// soft delete：行はまだ存在し、discarded_at に時刻が刻まれている。
		var discardedAt *time.Time
		if err := conn.QueryRow(ctx, `SELECT discarded_at FROM users WHERE id = $1`, victim).Scan(&discardedAt); err != nil {
			t.Fatalf("select discarded user: %v", err)
		}
		if discardedAt == nil {
			t.Fatal("users.discarded_at is NULL, want a timestamp")
		}
		// Rails parity：victim の review 自体は kept のままである。非表示化は
		// 純粋に読み取り側で行われる。
		var keptReviews int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE user_id = $1 AND discarded_at IS NULL`, victim).Scan(&keptReviews); err != nil {
			t.Fatalf("count victim reviews: %v", err)
		}
		if keptReviews != 2 {
			t.Errorf("victim kept reviews = %d, want 2 (reviews must not be discarded)", keptReviews)
		}
	})

	t.Run("2 回目の discard と存在しない id の discard は ErrUserNotFound になる", func(t *testing.T) {
		if err := repo.DiscardUser(ctx, victim); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrUserNotFound)
		}
		if err := repo.DiscardUser(ctx, uid.N(99999)); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrUserNotFound)
		}
	})
}
