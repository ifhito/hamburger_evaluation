package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

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
