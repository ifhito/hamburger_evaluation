package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestUserIdentityQuery は、外部のサービスのアカウントとの結び付きなどの読み取りを、実際の PostgreSQL に対して
// 検証する(書き込みの adapter/repository には依存しないので、データは SQL で直接入れる)。
func TestUserIdentityQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()

	t.Run("サービス上の ID で結び付きを引け、利用者ごとに結び付けた順に一覧できる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		if _, err := conn.Exec(ctx, `INSERT INTO user_identities (user_id, provider, provider_user_id, email) VALUES ($1, 'google', 'sub-1', 'alice@gmail.example')`, alice); err != nil {
			t.Fatal(err)
		}
		q := query.NewUserIdentityQuery(conn)

		got, err := q.GetIdentityByProviderUserID(ctx, domain.ProviderGoogle, "sub-1")
		if err != nil || got.UserID != alice || got.Email != "alice@gmail.example" || got.ProviderUserID != "sub-1" || !domain.IsUUID(got.ID) {
			t.Fatalf("GetIdentityByProviderUserID = %+v, %v", got, err)
		}
		list, err := q.ListIdentitiesByUser(ctx, alice)
		if err != nil || len(list) != 1 || list[0].ProviderUserID != "sub-1" {
			t.Fatalf("ListIdentitiesByUser = %+v, %v", list, err)
		}
		if _, err := q.GetIdentityByProviderUserID(ctx, domain.ProviderGoogle, "unknown"); !errors.Is(err, domain.ErrIdentityNotFound) {
			t.Fatalf("知らない ID = %v, want ErrIdentityNotFound", err)
		}
		if others, err := q.ListIdentitiesByUser(ctx, "00000000-0000-4000-8000-000000000000"); err != nil || len(others) != 0 {
			t.Fatalf("結び付きのない利用者の一覧 = %+v, %v, want 空", others, err)
		}
	})
}
