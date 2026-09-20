package repository_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// strPtr は、usecase.ProfileChanges のフィールド用に、s へのポインタを返す。
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

	created, err := repo.CreateUser(ctx, usecase.CreateUserParams{
		Username:       "alice",
		Email:          "alice@example.com",
		PasswordDigest: "digest-alice",
		Admin:          false,
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("CreateUser returned zero ID")
	}
	wantUser := domain.User{ID: created.ID, Username: "alice", Email: "alice@example.com", Admin: false}
	if created != wantUser {
		t.Fatalf("CreateUser = %+v, want %+v", created, wantUser)
	}

	t.Run("GetActiveUserByEmail は user と digest を返す", func(t *testing.T) {
		creds, err := repo.GetActiveUserByEmail(ctx, "alice@example.com")
		if err != nil {
			t.Fatalf("GetActiveUserByEmail returned error: %v", err)
		}
		if creds.User != wantUser {
			t.Fatalf("GetActiveUserByEmail user = %+v, want %+v", creds.User, wantUser)
		}
		if creds.PasswordDigest != "digest-alice" {
			t.Fatalf("GetActiveUserByEmail digest = %q, want %q", creds.PasswordDigest, "digest-alice")
		}
	})

	t.Run("GetActiveUserByID は user を返す", func(t *testing.T) {
		user, err := repo.GetActiveUserByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetActiveUserByID returned error: %v", err)
		}
		if user != wantUser {
			t.Fatalf("GetActiveUserByID = %+v, want %+v", user, wantUser)
		}
	})

	t.Run("存在しない email と id は ErrUserNotFound になる", func(t *testing.T) {
		if _, err := repo.GetActiveUserByEmail(ctx, "nobody@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByEmail error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if _, err := repo.GetActiveUserByID(ctx, created.ID+1000); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByID error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("email が重複すると ErrEmailTaken になる", func(t *testing.T) {
		_, err := repo.CreateUser(ctx, usecase.CreateUserParams{
			Username:       "alice2",
			Email:          "alice@example.com",
			PasswordDigest: "digest-alice2",
			Admin:          false,
		})
		if !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("CreateUser error = %v, want %v", err, domain.ErrEmailTaken)
		}
	})

	t.Run("discard 済みの user は active な検索から除外される", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE users SET discarded_at = now() WHERE id = $1", created.ID); err != nil {
			t.Fatalf("discard user: %v", err)
		}
		if _, err := repo.GetActiveUserByEmail(ctx, "alice@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByEmail error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if _, err := repo.GetActiveUserByID(ctx, created.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByID error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})
}

// TestUserRepositoryManagement は、S8 の user 管理の永続化を検証する。
// kept のみの一覧、カラム単位でトランザクションを伴うプロフィール更新、
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
	shopRepo := repository.NewShopRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, $3, $4) RETURNING id`
	alice := insertRow(ctx, t, conn, insertUser, "alice@example.com", "alice", "digest-alice", false)
	bob := insertRow(ctx, t, conn, insertUser, "bob@example.com", "bob", "digest-bob", false)
	victim := insertRow(ctx, t, conn, insertUser, "victim@example.com", "victim", "digest-victim", false)
	ghost := insertRow(ctx, t, conn, insertUser, "ghost@example.com", "ghost", "digest-ghost", false)
	if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, ghost); err != nil {
		t.Fatalf("discard ghost: %v", err)
	}

	// active な shop 1 つが 2 つの burger を提供している："shared" は victim と
	// alice の両方が review し、"solo" は victim だけが review した。victim の
	// discard 後、shared は alice の review だけに減り、solo は stats がゼロの
	// 行にならなければならない。
	shop := insertRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Active One", 1, nil, nil)
	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	shared := insertRow(ctx, t, conn, insertBurger, "Shared")
	solo := insertRow(ctx, t, conn, insertBurger, "Solo")
	for _, burgerID := range []int64{shared, solo} {
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shop, burgerID); err != nil {
			t.Fatalf("link shop %d burger %d: %v", shop, burgerID, err)
		}
	}
	victimShared := mustCreateReview(ctx, t, reviewRepo, 2, "meh", victim, shared)
	aliceShared := mustCreateReview(ctx, t, reviewRepo, 4, "good", alice, shared)
	victimSolo := mustCreateReview(ctx, t, reviewRepo, 5, "only mine", victim, solo)

	t.Run("ListActiveUsers は kept の user を id 順に返す", func(t *testing.T) {
		users, err := repo.ListActiveUsers(ctx)
		if err != nil {
			t.Fatalf("ListActiveUsers returned error: %v", err)
		}
		want := []domain.User{
			{ID: alice, Username: "alice", Email: "alice@example.com"},
			{ID: bob, Username: "bob", Email: "bob@example.com"},
			{ID: victim, Username: "victim", Email: "victim@example.com"},
		}
		if !reflect.DeepEqual(users, want) {
			t.Fatalf("ListActiveUsers = %+v, want %+v (ghost excluded, id ascending)", users, want)
		}
	})

	t.Run("UpdateUserProfile は指定されたフィールドだけを更新する", func(t *testing.T) {
		updated, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{Username: strPtr("bobby")})
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
		updated, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{
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
		user, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{})
		if err != nil {
			t.Fatalf("UpdateUserProfile returned error: %v", err)
		}
		if want := (domain.User{ID: bob, Username: "bobby", Email: "bobby@example.com"}); user != want {
			t.Fatalf("UpdateUserProfile = %+v, want unchanged %+v", user, want)
		}
	})

	t.Run("UpdateUserProfile で使用済みの email を指定すると ErrEmailTaken になり rollback される", func(t *testing.T) {
		_, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{
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
		for name, id := range map[string]int64{"unknown": 99999, "discarded": ghost} {
			if _, err := repo.UpdateUserProfile(ctx, id, usecase.ProfileChanges{Username: strPtr("x")}); !errors.Is(err, domain.ErrUserNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrUserNotFound)
			}
			if _, err := repo.UpdateUserProfile(ctx, id, usecase.ProfileChanges{}); !errors.Is(err, domain.ErrUserNotFound) {
				t.Errorf("%s (zero changes): error = %v, want %v", name, err, domain.ErrUserNotFound)
			}
		}
	})

	t.Run("DiscardUser は user に discard 時刻を刻み、その user が review した burger の stats を再計算する", func(t *testing.T) {
		if got := requireConsistentStats(ctx, t, conn, shared); got.ReviewCount != 2 {
			t.Fatalf("shared stats before discard = %+v, want count 2", got)
		}
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
		if _, err := repo.GetActiveUserByID(ctx, victim); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("GetActiveUserByID after discard = %v, want %v", err, domain.ErrUserNotFound)
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
		// shared は alice の review だけに減る。alice の review は
		// 影響を受けない。
		sharedStats := requireConsistentStats(ctx, t, conn, shared)
		if sharedStats.ReviewCount != 1 || sharedStats.AverageRating != 4.0 {
			t.Errorf("shared stats after discard = %+v, want only alice's rating 4", sharedStats)
		}
		// solo は、victim だけが review したので、ゼロの行になる。
		soloStats := requireConsistentStats(ctx, t, conn, solo)
		want := storedBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: soloStats.CalculatedAt}
		if soloStats != want {
			t.Errorf("solo stats after discard = %+v, want the zero row", soloStats)
		}
	})

	t.Run("2 回目の discard と存在しない id の discard は ErrUserNotFound になる", func(t *testing.T) {
		if err := repo.DiscardUser(ctx, victim); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrUserNotFound)
		}
		if err := repo.DiscardUser(ctx, 99999); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("読み取り経路は discard 済みの user の kept な review を隠す", func(t *testing.T) {
		// フィード：alice の review だけが残り、表示される stats は再計算された
		// burger_stats の行（count 1）と一致する。
		feed, err := reviewRepo.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(feed), []int64{aliceShared.ID}; !reflect.DeepEqual(got, want) {
			t.Fatalf("feed ids = %v, want %v (victim's reviews hidden)", got, want)
		}
		sharedStats := requireConsistentStats(ctx, t, conn, shared)
		if feed[0].Burger.ReviewCount != sharedStats.ReviewCount || feed[0].Burger.ReviewCount != int64(len(feed)) {
			t.Errorf("displayed review count = %d, want burger_stats %d = %d displayed reviews",
				feed[0].Burger.ReviewCount, sharedStats.ReviewCount, len(feed))
		}
		// 詳細：discard 済みの author の review は、存在しない review と
		// 区別がつかない。alice の review には引き続き到達できる。
		if _, err := reviewRepo.GetReview(ctx, victimShared.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim shared) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := reviewRepo.GetReview(ctx, victimSolo.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim solo) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := reviewRepo.GetReview(ctx, aliceShared.ID); err != nil {
			t.Errorf("GetReview(alice) returned error: %v", err)
		}
		// shop の review：alice の review だけが一覧に載る。
		shopReviews, err := shopRepo.ListShopReviews(ctx, shop)
		if err != nil {
			t.Fatalf("ListShopReviews returned error: %v", err)
		}
		ids := make([]int64, 0, len(shopReviews))
		for _, r := range shopReviews {
			ids = append(ids, r.ID)
		}
		if want := []int64{aliceShared.ID}; !reflect.DeepEqual(ids, want) {
			t.Errorf("shop review ids = %v, want %v (victim's review hidden)", ids, want)
		}
	})
}
