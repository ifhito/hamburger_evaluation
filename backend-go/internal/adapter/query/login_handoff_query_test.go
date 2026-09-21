package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestLoginHandoffQuery は、画面へ渡すコードの中身の読み取りを、実際の PostgreSQL に対して検証する
// (書き込みの adapter/repository には依存しないので、データは SQL で直接入れる)。
func TestLoginHandoffQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()

	t.Run("期限内のコードの中身(結果・利用者・戻り先・結び付けの値のハッシュ)を返す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		if _, err := conn.Exec(ctx, `INSERT INTO login_handoffs (code_hash, binder_hash, outcome, user_id, return_to, expires_at)
			VALUES ('code-hash', 'binder-hash', 'signed_in', $1, '/shops', now() + interval '1 minute')`, alice); err != nil {
			t.Fatal(err)
		}

		got, err := query.NewLoginHandoffQuery(conn).GetLoginHandoffByCodeHash(ctx, "code-hash")

		if err != nil || got.Outcome != domain.OutcomeSignedIn || got.UserID != alice || got.ReturnTo != "/shops" || got.BinderHash != "binder-hash" || !domain.IsUUID(got.ID) {
			t.Fatalf("GetLoginHandoffByCodeHash = %+v, %v", got, err)
		}
	})

	t.Run("期限切れ・知らないコードは、無効として返す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		if _, err := conn.Exec(ctx, `INSERT INTO login_handoffs (code_hash, binder_hash, outcome, expires_at) VALUES ('old', 'b', 'failed', now() - interval '1 second')`); err != nil {
			t.Fatal(err)
		}
		q := query.NewLoginHandoffQuery(conn)
		for _, hash := range []string{"old", "unknown"} {
			if _, err := q.GetLoginHandoffByCodeHash(ctx, hash); !errors.Is(err, domain.ErrLoginHandoffInvalid) {
				t.Errorf("%s = %v, want ErrLoginHandoffInvalid", hash, err)
			}
		}
	})
}
