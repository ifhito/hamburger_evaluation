package repository_test

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

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// migrationsDir points at db/migrations relative to this package.
const migrationsDir = "../../../db/migrations"

// TestUserRepository exercises the repository against a real PostgreSQL,
// following the db/migrations_test.go pattern: a per-run database is
// created inside the compose Postgres instance, migrated up, and dropped
// afterwards. It requires TEST_DATABASE_URL to point at a maintenance
// database whose user may create and drop databases.
func TestUserRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping DB-backed repository test")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to admin database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	dbName := fmt.Sprintf("hamburger_evaluation_go_repo_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatalf("create test database: %v", err)
	}
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

	applyUpMigrations(ctx, t, conn)

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

// applyUpMigrations applies all *.up.sql files in ascending order.
func applyUpMigrations(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var ups []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			ups = append(ups, filepath.Join(migrationsDir, entry.Name()))
		}
	}
	if len(ups) == 0 {
		t.Fatal("no up migrations found")
	}
	sort.Strings(ups)
	for _, file := range ups {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
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
