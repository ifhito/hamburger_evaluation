package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestLoginHandoffRepository は、「画面へ渡すコード」の書き込みを、実際の PostgreSQL に対して検証する。
func TestLoginHandoffRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()

	// 期限内のコードは、ロックできて(行を排他ロックする)、削除すると、2 度とロックできない。読み取りは adapter/query が担い、
	// このテストは依存しないので、内容は SQL で直接確かめる。
	t.Run("保存したコードは、ロックして削除でき、削除したあとは無効になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewLoginHandoffRepository(conn)
		if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{BinderHash: "binder-hash", CodeHash: "hash-1", Outcome: domain.OutcomeSignedIn, UserID: alice, ReturnTo: "/shops"}); err != nil {
			t.Fatal(err)
		}
		if err := repo.LockLoginHandoff(ctx, "hash-1"); err != nil {
			t.Fatalf("ロック: %v", err)
		}
		var id, outcome, returnTo string
		var userID *string
		if err := conn.QueryRow(ctx, `SELECT id, outcome, user_id, return_to FROM login_handoffs WHERE code_hash = 'hash-1'`).Scan(&id, &outcome, &userID, &returnTo); err != nil {
			t.Fatal(err)
		}
		if !domain.IsUUID(id) || outcome != "signed_in" || userID == nil || *userID != alice || returnTo != "/shops" {
			t.Fatalf("保存された内容: %s %s %v %s", id, outcome, userID, returnTo)
		}
		if err := repo.DiscardLoginHandoff(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := repo.LockLoginHandoff(ctx, "hash-1"); !errors.Is(err, domain.ErrLoginHandoffInvalid) {
			t.Fatalf("削除後のロック = %v, want ErrLoginHandoffInvalid", err)
		}
	})

	t.Run("利用者を伴わない結果は、利用者なしで保存できる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewLoginHandoffRepository(conn)
		if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{BinderHash: "binder-hash", CodeHash: "hash-2", Outcome: domain.OutcomeAccountExists}); err != nil {
			t.Fatal(err)
		}
		var userID *string
		if err := conn.QueryRow(ctx, `SELECT user_id FROM login_handoffs WHERE code_hash = 'hash-2'`).Scan(&userID); err != nil || userID != nil {
			t.Fatalf("user_id = %v, err = %v, want NULL", userID, err)
		}
	})

	t.Run("期限切れのコードはロックできず、掃除で消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewLoginHandoffRepository(conn)
		for _, h := range []string{"old-1", "old-2", "fresh"} {
			if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{BinderHash: "binder-hash", CodeHash: h, Outcome: domain.OutcomeFailed}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(ctx, `UPDATE login_handoffs SET expires_at = now() - interval '1 second' WHERE code_hash LIKE 'old-%'`); err != nil {
			t.Fatal(err)
		}
		if err := repo.LockLoginHandoff(ctx, "old-1"); !errors.Is(err, domain.ErrLoginHandoffInvalid) {
			t.Fatalf("期限切れ = %v, want ErrLoginHandoffInvalid", err)
		}
		n, err := repo.DiscardExpiredLoginHandoffs(ctx, 10)
		if err != nil || n != 2 {
			t.Fatalf("掃除 = %d, %v, want 2 件", n, err)
		}
		if err := repo.LockLoginHandoff(ctx, "fresh"); err != nil {
			t.Fatalf("期限内のコードが消えた: %v", err)
		}
	})

	t.Run("掃除は、上限の件数までしか消さない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewLoginHandoffRepository(conn)
		for _, h := range []string{"a", "b", "c"} {
			if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{BinderHash: "binder-hash", CodeHash: h, Outcome: domain.OutcomeFailed}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(ctx, `UPDATE login_handoffs SET expires_at = now() - interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		if n, err := repo.DiscardExpiredLoginHandoffs(ctx, 2); err != nil || n != 2 {
			t.Fatalf("上限 2 の掃除 = %d, %v", n, err)
		}
		if n := countRows(ctx, t, conn, "login_handoffs"); n != 1 {
			t.Fatalf("残り = %d, want 1", n)
		}
	})

	t.Run("結果の種類と利用者の有無が食い違う行・知らない種類(廃止した結び付けの開始を含む)は、DB の制約で保存できない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewLoginHandoffRepository(conn)
		cases := map[string]domain.CreateLoginHandoffParams{
			"サインインの成功なのに利用者がない": {BinderHash: "binder-hash", CodeHash: "x1", Outcome: domain.OutcomeSignedIn},
			"失敗なのに利用者がある":       {BinderHash: "binder-hash", CodeHash: "x2", Outcome: domain.OutcomeFailed, UserID: alice},
			"知らない種類":            {BinderHash: "binder-hash", CodeHash: "x3", Outcome: domain.LoginHandoffOutcome("hacked")},
			"結び付けの値が空":          {CodeHash: "x5", Outcome: domain.OutcomeFailed},
			"廃止した結び付けの開始の種類":    {BinderHash: "binder-hash", CodeHash: "x4", Outcome: domain.LoginHandoffOutcome("link_intent"), UserID: alice},
		}
		for name, p := range cases {
			if err := repo.CreateLoginHandoff(ctx, p); err == nil {
				t.Errorf("%s: 保存できてしまった", name)
			}
		}
		// domain が定義する種類は、すべて保存できる(DB の CHECK と食い違わない)。
		for _, o := range []domain.LoginHandoffOutcome{
			domain.OutcomeSignedIn, domain.OutcomeLinked, domain.OutcomeAccountExists,
			domain.OutcomeIdentityTaken, domain.OutcomeAlreadyLinked, domain.OutcomeFailed,
		} {
			user := ""
			if o.NeedsUser() {
				user = alice
			}
			if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{BinderHash: "binder-hash", CodeHash: "ok-" + string(o), Outcome: o, UserID: user}); err != nil {
				t.Errorf("種類 %q を保存できない: %v", o, err)
			}
		}
	})

	t.Run("コードを使う手順の途中(ロックしたトランザクション)では、同じコードを扱う別のトランザクションは待たされ、削除して確定すると無効になる", func(t *testing.T) {
		_, dbURL := dbtest.New(t)
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		if err := repository.NewLoginHandoffRepository(pool).CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{BinderHash: "binder-hash", CodeHash: "race", Outcome: domain.OutcomeFailed}); err != nil {
			t.Fatal(err)
		}
		tx1, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx1.Rollback(ctx) }()
		if err := repository.NewLoginHandoffRepository(tx1).LockLoginHandoff(ctx, "race"); err != nil {
			t.Fatal(err)
		}
		var id string
		if err := tx1.QueryRow(ctx, `SELECT id FROM login_handoffs WHERE code_hash = 'race'`).Scan(&id); err != nil {
			t.Fatal(err)
		}

		secondDone := make(chan error, 1)
		go func() {
			tx2, err := pool.Begin(ctx)
			if err != nil {
				secondDone <- err
				return
			}
			defer func() { _ = tx2.Rollback(ctx) }()
			secondDone <- repository.NewLoginHandoffRepository(tx2).LockLoginHandoff(ctx, "race")
		}()
		select {
		case err := <-secondDone:
			t.Fatalf("先のトランザクションが終わる前に、2 つ目がロックを取れた: %v", err)
		case <-time.After(300 * time.Millisecond):
		}
		if err := repository.NewLoginHandoffRepository(tx1).DiscardLoginHandoff(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := tx1.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-secondDone; !errors.Is(err, domain.ErrLoginHandoffInvalid) {
			t.Fatalf("2 つ目 = %v, want ErrLoginHandoffInvalid(行が消えている)", err)
		}
	})
}
