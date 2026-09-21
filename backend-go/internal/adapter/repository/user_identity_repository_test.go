package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

func insertTestUser(ctx context.Context, t *testing.T, conn *pgx.Conn, email string) string {
	t.Helper()
	return dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ($1, 'user', 'd') RETURNING id`, email)
}

// TestUserIdentityRepository は、外部のサービスのアカウントとの結び付きの書き込みを、実際の PostgreSQL に対して
// 検証する(読み取りは adapter/query が担い、このテストは依存しないので、結果は SQL で直接確かめる)。
func TestUserIdentityRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()

	t.Run("結び付けると、保存した内容が返り、DB に 1 行できる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		repo := repository.NewUserIdentityRepository(conn)

		created, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "alice@gmail.example"})
		if err != nil || !domain.IsUUID(created.ID) || created.UserID != alice || created.ProviderUserID != "sub-1" || created.Email != "alice@gmail.example" || created.CreatedAt.IsZero() {
			t.Fatalf("created = %+v, err = %v", created, err)
		}
		if n := countRows(ctx, t, conn, "user_identities"); n != 1 {
			t.Fatalf("結び付きが %d 行", n)
		}
	})

	t.Run("同じ外部のアカウントは、別の利用者に結び付けられない(ErrIdentityTaken)", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		bob := insertTestUser(ctx, t, conn, "bob@example.com")
		repo := repository.NewUserIdentityRepository(conn)
		if _, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "a@x.example"}); err != nil {
			t.Fatal(err)
		}
		_, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: bob, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "b@x.example"})
		if !errors.Is(err, domain.ErrIdentityTaken) {
			t.Fatalf("err = %v, want ErrIdentityTaken", err)
		}
	})

	t.Run("1 つのアカウントには、同じサービスの結び付きを 2 つ作れない(ErrIdentityAlreadyLinked)", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		repo := repository.NewUserIdentityRepository(conn)
		if _, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "a@x.example"}); err != nil {
			t.Fatal(err)
		}
		_, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "sub-2", Email: "a@x.example"})
		if !errors.Is(err, domain.ErrIdentityAlreadyLinked) {
			t.Fatalf("err = %v, want ErrIdentityAlreadyLinked", err)
		}
	})

	t.Run("解除すると行が消え、もう一度解除すると ErrIdentityNotFound になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		repo := repository.NewUserIdentityRepository(conn)
		if _, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "a@x.example"}); err != nil {
			t.Fatal(err)
		}
		if err := repo.DiscardUserIdentity(ctx, alice, domain.ProviderGoogle); err != nil {
			t.Fatal(err)
		}
		if n := countRows(ctx, t, conn, "user_identities"); n != 0 {
			t.Fatalf("解除したのに %d 行残っている", n)
		}
		if err := repo.DiscardUserIdentity(ctx, alice, domain.ProviderGoogle); !errors.Is(err, domain.ErrIdentityNotFound) {
			t.Fatalf("2 回目の解除 = %v, want ErrIdentityNotFound", err)
		}
	})

	t.Run("解除は、指定した利用者の結び付きだけを消す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		bob := insertTestUser(ctx, t, conn, "bob@example.com")
		repo := repository.NewUserIdentityRepository(conn)
		for user, sub := range map[string]string{alice: "sub-a", bob: "sub-b"} {
			if _, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: user, Provider: domain.ProviderGoogle, ProviderUserID: sub, Email: "x@x.example"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.DiscardUserIdentity(ctx, alice, domain.ProviderGoogle); err != nil {
			t.Fatal(err)
		}
		var remaining string
		if err := conn.QueryRow(ctx, `SELECT user_id FROM user_identities`).Scan(&remaining); err != nil || remaining != bob {
			t.Fatalf("残った結び付きの持ち主 = %q, %v, want bob", remaining, err)
		}
	})

	t.Run("知らないサービスの名前・空の ID は、DB の制約で保存できない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		repo := repository.NewUserIdentityRepository(conn)
		cases := map[string]domain.CreateUserIdentityParams{
			"知らないサービス":     {UserID: alice, Provider: "twitter", ProviderUserID: "s", Email: "a@x.example"},
			"サービス上の ID が空": {UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "", Email: "a@x.example"},
		}
		for name, p := range cases {
			if _, err := repo.CreateUserIdentity(ctx, p); err == nil {
				t.Errorf("%s: 保存できてしまった", name)
			}
		}
	})

	t.Run("利用者の行を消すと、結び付きも連鎖して消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertTestUser(ctx, t, conn, "alice@example.com")
		repo := repository.NewUserIdentityRepository(conn)
		if _, err := repo.CreateUserIdentity(ctx, domain.CreateUserIdentityParams{UserID: alice, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "a@x.example"}); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `DELETE FROM users WHERE id = $1`, alice); err != nil {
			t.Fatal(err)
		}
		if n := countRows(ctx, t, conn, "user_identities"); n != 0 {
			t.Fatalf("結び付きが %d 件残っている", n)
		}
	})

	t.Run("パスワードなしの利用者を作れる(空文字列の digest は NULL として保存され、DB には空文字列を入れられない)", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		users := repository.NewUserRepository(conn)
		none, err := users.CreateUser(ctx, domain.CreateUserParams{Email: "g@example.com", Username: "g"})
		if err != nil {
			t.Fatalf("パスワードなしの作成: %v", err)
		}
		var isNull bool
		if err := conn.QueryRow(ctx, `SELECT password_digest IS NULL FROM users WHERE id = $1`, none.ID).Scan(&isNull); err != nil || !isNull {
			t.Fatalf("password_digest が NULL でない: %v %v", isNull, err)
		}
		if _, err := conn.Exec(ctx, `UPDATE users SET password_digest = '' WHERE id = $1`, none.ID); err == nil {
			t.Fatal("空文字列の digest を保存できてしまった")
		}
	})
}
