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

// strPtr returns a pointer to s, for usecase.ProfileChanges fields.
func strPtr(s string) *string { return &s }

// TestUserRepository exercises the repository against a real PostgreSQL,
// using the shared dbtest scaffold: a per-run database is created inside
// the compose Postgres instance, migrated up, and dropped afterwards. It
// requires TEST_DATABASE_URL to point at a maintenance database whose user
// may create and drop databases; without it the test skips (inside
// dbtest.New).
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

	t.Run("GetActiveUserByEmail returns user and digest", func(t *testing.T) {
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

	t.Run("GetActiveUserByID returns user", func(t *testing.T) {
		user, err := repo.GetActiveUserByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetActiveUserByID returned error: %v", err)
		}
		if user != wantUser {
			t.Fatalf("GetActiveUserByID = %+v, want %+v", user, wantUser)
		}
	})

	t.Run("unknown email and id yield ErrUserNotFound", func(t *testing.T) {
		if _, err := repo.GetActiveUserByEmail(ctx, "nobody@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByEmail error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if _, err := repo.GetActiveUserByID(ctx, created.ID+1000); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByID error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("duplicate email yields ErrEmailTaken", func(t *testing.T) {
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

	t.Run("discarded user is excluded from active lookups", func(t *testing.T) {
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

// TestUserRepositoryManagement exercises the S8 user-management
// persistence: the kept-only listing, the column-scoped transactional
// profile update, the soft delete with same-transaction burger_stats
// recalculation, and the read-side exclusion of discarded users' reviews
// from the feed, the review detail, and the shop reviews.
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

	// One active shop serving two burgers: "shared" is reviewed by the
	// victim AND alice, "solo" only by the victim — after the victim's
	// discard, shared must drop to alice's review alone and solo must go
	// to the zero stats row.
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

	t.Run("ListActiveUsers returns kept users ordered by id", func(t *testing.T) {
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

	t.Run("UpdateUserProfile applies only the present fields", func(t *testing.T) {
		updated, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{Username: strPtr("bobby")})
		if err != nil {
			t.Fatalf("UpdateUserProfile returned error: %v", err)
		}
		if want := (domain.User{ID: bob, Username: "bobby", Email: "bob@example.com"}); updated != want {
			t.Fatalf("UpdateUserProfile = %+v, want %+v", updated, want)
		}
		// The untouched columns are untouched in storage too.
		var email, digest string
		if err := conn.QueryRow(ctx, `SELECT email, password_digest FROM users WHERE id = $1`, bob).Scan(&email, &digest); err != nil {
			t.Fatalf("select bob: %v", err)
		}
		if email != "bob@example.com" || digest != "digest-bob" {
			t.Fatalf("stored (email, digest) = (%q, %q), want unchanged", email, digest)
		}
	})

	t.Run("UpdateUserProfile applies several fields in one transaction", func(t *testing.T) {
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

	t.Run("UpdateUserProfile with zero changes returns the current user", func(t *testing.T) {
		user, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{})
		if err != nil {
			t.Fatalf("UpdateUserProfile returned error: %v", err)
		}
		if want := (domain.User{ID: bob, Username: "bobby", Email: "bobby@example.com"}); user != want {
			t.Fatalf("UpdateUserProfile = %+v, want unchanged %+v", user, want)
		}
	})

	t.Run("UpdateUserProfile taken email yields ErrEmailTaken and rolls back", func(t *testing.T) {
		_, err := repo.UpdateUserProfile(ctx, bob, usecase.ProfileChanges{
			Username: strPtr("sneaky"),
			Email:    strPtr("alice@example.com"),
		})
		if !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("UpdateUserProfile error = %v, want %v", err, domain.ErrEmailTaken)
		}
		// The username change of the same call was rolled back with it.
		var username string
		if err := conn.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, bob).Scan(&username); err != nil {
			t.Fatalf("select bob: %v", err)
		}
		if username != "bobby" {
			t.Fatalf("stored username = %q, want the pre-call %q (rollback)", username, "bobby")
		}
	})

	t.Run("UpdateUserProfile unknown and discarded ids yield ErrUserNotFound", func(t *testing.T) {
		for name, id := range map[string]int64{"unknown": 99999, "discarded": ghost} {
			if _, err := repo.UpdateUserProfile(ctx, id, usecase.ProfileChanges{Username: strPtr("x")}); !errors.Is(err, domain.ErrUserNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrUserNotFound)
			}
			if _, err := repo.UpdateUserProfile(ctx, id, usecase.ProfileChanges{}); !errors.Is(err, domain.ErrUserNotFound) {
				t.Errorf("%s (zero changes): error = %v, want %v", name, err, domain.ErrUserNotFound)
			}
		}
	})

	t.Run("DiscardUser stamps the user and recalculates its burgers' stats", func(t *testing.T) {
		if got := requireConsistentStats(ctx, t, conn, shared); got.ReviewCount != 2 {
			t.Fatalf("shared stats before discard = %+v, want count 2", got)
		}
		if err := repo.DiscardUser(ctx, victim); err != nil {
			t.Fatalf("DiscardUser returned error: %v", err)
		}
		// Soft delete: the row still exists with discarded_at stamped.
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
		// Rails parity: the victim's reviews themselves stay kept — hiding
		// is purely read-side.
		var keptReviews int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE user_id = $1 AND discarded_at IS NULL`, victim).Scan(&keptReviews); err != nil {
			t.Fatalf("count victim reviews: %v", err)
		}
		if keptReviews != 2 {
			t.Errorf("victim kept reviews = %d, want 2 (reviews must not be discarded)", keptReviews)
		}
		// shared drops to alice's review alone; alice's review is unaffected.
		sharedStats := requireConsistentStats(ctx, t, conn, shared)
		if sharedStats.ReviewCount != 1 || sharedStats.AverageRating != 4.0 {
			t.Errorf("shared stats after discard = %+v, want only alice's rating 4", sharedStats)
		}
		// solo, reviewed only by the victim, goes to the zero row.
		soloStats := requireConsistentStats(ctx, t, conn, solo)
		want := storedBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: soloStats.CalculatedAt}
		if soloStats != want {
			t.Errorf("solo stats after discard = %+v, want the zero row", soloStats)
		}
	})

	t.Run("second and unknown discards yield ErrUserNotFound", func(t *testing.T) {
		if err := repo.DiscardUser(ctx, victim); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrUserNotFound)
		}
		if err := repo.DiscardUser(ctx, 99999); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("read paths hide the discarded user's kept reviews", func(t *testing.T) {
		// Feed: only alice's review remains, and its displayed stats match
		// the recalculated burger_stats row (count 1).
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
		// Detail: the discarded author's review is indistinguishable from a
		// missing one; alice's stays reachable.
		if _, err := reviewRepo.GetReview(ctx, victimShared.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim shared) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := reviewRepo.GetReview(ctx, victimSolo.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim solo) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := reviewRepo.GetReview(ctx, aliceShared.ID); err != nil {
			t.Errorf("GetReview(alice) returned error: %v", err)
		}
		// Shop reviews: only alice's review is listed.
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
