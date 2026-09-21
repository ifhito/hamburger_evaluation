package repository_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

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

	t.Run("保存したコードは、1 回だけ取り出せて、その内容を返し、2 回目は無効になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewLoginHandoffRepository(conn)
		if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{CodeHash: "hash-1", Outcome: domain.OutcomeSignedIn, UserID: alice, ReturnTo: "/shops"}); err != nil {
			t.Fatal(err)
		}
		got, err := repo.DiscardLoginHandoff(ctx, "hash-1")
		if err != nil || got.Outcome != domain.OutcomeSignedIn || got.UserID != alice || got.ReturnTo != "/shops" || !domain.IsUUID(got.ID) {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
		if _, err := repo.DiscardLoginHandoff(ctx, "hash-1"); !errors.Is(err, domain.ErrLoginHandoffInvalid) {
			t.Fatalf("2 回目 = %v, want ErrLoginHandoffInvalid", err)
		}
	})

	t.Run("利用者を伴わない結果は、利用者なしで保存・取り出しできる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewLoginHandoffRepository(conn)
		if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{CodeHash: "hash-2", Outcome: domain.OutcomeAccountExists}); err != nil {
			t.Fatal(err)
		}
		got, err := repo.DiscardLoginHandoff(ctx, "hash-2")
		if err != nil || got.Outcome != domain.OutcomeAccountExists || got.UserID != "" || got.ReturnTo != "" {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
	})

	t.Run("期限切れのコードは取り出せず、掃除で消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewLoginHandoffRepository(conn)
		for _, h := range []string{"old-1", "old-2", "fresh"} {
			if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{CodeHash: h, Outcome: domain.OutcomeFailed}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(ctx, `UPDATE login_handoffs SET expires_at = now() - interval '1 second' WHERE code_hash LIKE 'old-%'`); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.DiscardLoginHandoff(ctx, "old-1"); !errors.Is(err, domain.ErrLoginHandoffInvalid) {
			t.Fatalf("期限切れ = %v, want ErrLoginHandoffInvalid", err)
		}
		n, err := repo.DiscardExpiredLoginHandoffs(ctx, 10)
		if err != nil || n != 2 {
			t.Fatalf("掃除 = %d, %v, want 2 件(old-1 は取り出しに失敗しても行は残っている)", n, err)
		}
		if _, err := repo.DiscardLoginHandoff(ctx, "fresh"); err != nil {
			t.Fatalf("期限内のコードが消えた: %v", err)
		}
	})

	t.Run("掃除は、上限の件数までしか消さない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewLoginHandoffRepository(conn)
		for _, h := range []string{"a", "b", "c"} {
			if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{CodeHash: h, Outcome: domain.OutcomeFailed}); err != nil {
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

	t.Run("結果の種類と利用者の有無が食い違う行・知らない種類は、DB の制約で保存できない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewLoginHandoffRepository(conn)
		for name, p := range map[string]domain.CreateLoginHandoffParams{
			"サインインの成功なのに利用者がない": {CodeHash: "x1", Outcome: domain.OutcomeSignedIn},
			"失敗なのに利用者がある":       {CodeHash: "x2", Outcome: domain.OutcomeFailed, UserID: alice},
			"知らない種類":            {CodeHash: "x3", Outcome: domain.LoginHandoffOutcome("hacked")},
		} {
			if err := repo.CreateLoginHandoff(ctx, p); err == nil {
				t.Errorf("%s: 保存できてしまった", name)
			}
		}
		// domain が定義する種類は、すべて保存できる(DB の CHECK と食い違わない)。
		for _, o := range []domain.LoginHandoffOutcome{
			domain.OutcomeSignedIn, domain.OutcomeLinked, domain.OutcomeLinkIntent, domain.OutcomeAccountExists,
			domain.OutcomeIdentityTaken, domain.OutcomeAlreadyLinked, domain.OutcomeFailed,
		} {
			user := ""
			if o.NeedsUser() {
				user = alice
			}
			if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{CodeHash: "ok-" + string(o), Outcome: o, UserID: user}); err != nil {
				t.Errorf("種類 %q を保存できない: %v", o, err)
			}
		}
	})

	t.Run("同じコードを並行して使っても、成功するのは 1 回だけである", func(t *testing.T) {
		conn, dbURL := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		repo := repository.NewLoginHandoffRepository(pool)
		if err := repo.CreateLoginHandoff(ctx, domain.CreateLoginHandoffParams{CodeHash: "race", Outcome: domain.OutcomeSignedIn, UserID: alice}); err != nil {
			t.Fatal(err)
		}
		var ok, invalid atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := repo.DiscardLoginHandoff(ctx, "race"); err == nil {
					ok.Add(1)
				} else if errors.Is(err, domain.ErrLoginHandoffInvalid) {
					invalid.Add(1)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 1 || invalid.Load() != 15 {
			t.Fatalf("成功 %d 回・無効 %d 回, want 成功 1 回・無効 15 回", ok.Load(), invalid.Load())
		}
	})
}
