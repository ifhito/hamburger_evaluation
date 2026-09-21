package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestBurgerStatRepositoryRecalcRequests は、統計の再計算の依頼(burger_stats_recalc_requests)の登録・比較つきの
// 削除・失敗の記録を、実際の PostgreSQL に対して検証する。比較つきの削除は、「取り出したときの
// 番号(version)と今の番号が同じときだけ消す」削除で、再計算の最中に入った新しい依頼を消さないための
// ものである。TEST_DATABASE_URL がなければ、dbtest.New の内部で skip される。
func TestBurgerStatRepositoryRecalcRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	newBurger := func(t *testing.T, conn *pgx.Conn, name string) string {
		t.Helper()
		return dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name)
	}

	t.Run("登録すると、失敗の記録のない依頼が 1 行できる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")

		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		got, ok := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if !ok {
			t.Fatal("依頼の行がない")
		}
		if got.Attempts != 0 || got.NextAttemptAt != nil || got.LastError != nil {
			t.Errorf("登録した依頼 = %+v, want 失敗の記録がない", got)
		}
	})

	t.Run("同じバーガーに登録を重ねても 1 行のままで、version が進み、失敗の記録は消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		first, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		failure := domain.NewRecalcFailure(3, context.DeadlineExceeded, time.Now())
		if ok, err := repo.UpdateBurgerStatRecalcFailure(ctx, burger, first.Version, failure); err != nil || !ok {
			t.Fatalf("失敗の記録 = (%v, %v), want (true, nil)", ok, err)
		}

		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録を重ねる: %v", err)
		}
		got, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if got.Version <= first.Version {
			t.Errorf("version = %d, want %d より大きい", got.Version, first.Version)
		}
		if got.Attempts != 0 || got.NextAttemptAt != nil || got.LastError != nil {
			t.Errorf("登録を重ねた後の依頼 = %+v, want 失敗の記録が消えている", got)
		}
		if n := countRows(ctx, t, conn, "burger_stats_recalc_requests"); n != 1 {
			t.Errorf("行数 = %d, want 1", n)
		}
	})

	t.Run("存在しないバーガーは登録できず、依頼の行も残らない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		if err := repo.CreateBurgerStatRecalcRequest(ctx, uid.N(999)); err == nil {
			t.Fatal("存在しないバーガーの登録が成功した")
		}
		if n := countRows(ctx, t, conn, "burger_stats_recalc_requests"); n != 0 {
			t.Errorf("行数 = %d, want 0", n)
		}
	})

	t.Run("取り出したときの version と同じときだけ依頼を消せる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		taken, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)

		// 再計算の最中に、新しい書き込みが入って、依頼が登録し直された。
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録し直し: %v", err)
		}
		if ok, err := repo.DiscardBurgerStatRecalcRequest(ctx, burger, taken.Version); err != nil || ok {
			t.Fatalf("古い version での削除 = (%v, %v), want (false, nil)", ok, err)
		}
		latest, ok := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if !ok {
			t.Fatal("古い version での削除で、新しい依頼が消えた")
		}

		if ok, err := repo.DiscardBurgerStatRecalcRequest(ctx, burger, latest.Version); err != nil || !ok {
			t.Fatalf("最新の version での削除 = (%v, %v), want (true, nil)", ok, err)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 0 {
			t.Errorf("削除した後の依頼の数 = %d, want 0", n)
		}
		if ok, err := repo.DiscardBurgerStatRecalcRequest(ctx, burger, latest.Version); err != nil || ok {
			t.Errorf("すでにない依頼の削除 = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("消して作り直した依頼は、消す前の version に戻らないので、古い再計算に消されない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		taken, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		// 別のワーカーが、同じ依頼を先に終えて消し、そのあとで、新しい書き込みが依頼を作り直した。
		if ok, _ := repo.DiscardBurgerStatRecalcRequest(ctx, burger, taken.Version); !ok {
			t.Fatal("先に終えたワーカーの削除に失敗した")
		}
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("作り直し: %v", err)
		}

		if ok, err := repo.DiscardBurgerStatRecalcRequest(ctx, burger, taken.Version); err != nil || ok {
			t.Fatalf("古い再計算による削除 = (%v, %v), want (false, nil)", ok, err)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 1 {
			t.Errorf("作り直した依頼の数 = %d, want 1", n)
		}
	})

	t.Run("失敗を記録すると、回数が増え、次の再試行の時刻と理由が入る。version は変わらない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		before, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)

		next := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
		for i, reason := range []string{"first", "second"} {
			ok, err := repo.UpdateBurgerStatRecalcFailure(ctx, burger, before.Version, domain.RecalcFailure{NextAttemptAt: next, Reason: reason})
			if err != nil || !ok {
				t.Fatalf("失敗の記録 #%d = (%v, %v), want (true, nil)", i+1, ok, err)
			}
		}
		got, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if got.Version != before.Version {
			t.Errorf("version = %d, want %d(失敗の記録では進めない)", got.Version, before.Version)
		}
		if got.Attempts != 2 {
			t.Errorf("回数 = %d, want 2", got.Attempts)
		}
		if got.NextAttemptAt == nil || !got.NextAttemptAt.Equal(next) {
			t.Errorf("次の再試行の時刻 = %v, want %v", got.NextAttemptAt, next)
		}
		if got.LastError == nil || *got.LastError != "second" {
			t.Errorf("理由 = %v, want 直近の \"second\"", got.LastError)
		}
	})

	t.Run("version が違う依頼には、失敗を記録しない(その間に新しい書き込みが入っている)", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		taken, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録し直し: %v", err)
		}

		ok, err := repo.UpdateBurgerStatRecalcFailure(ctx, burger, taken.Version, domain.NewRecalcFailure(1, context.Canceled, time.Now()))
		if err != nil || ok {
			t.Fatalf("古い version への失敗の記録 = (%v, %v), want (false, nil)", ok, err)
		}
		got, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if got.Attempts != 0 || got.NextAttemptAt != nil || got.LastError != nil {
			t.Errorf("依頼 = %+v, want 触られていない", got)
		}
	})

	t.Run("文字数の上限ちょうどの理由は記録でき、上限を超える理由は拒否される", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewBurgerStatRepository(conn)
		burger := newBurger(t, conn, "Cheese")
		if err := repo.CreateBurgerStatRecalcRequest(ctx, burger); err != nil {
			t.Fatalf("登録: %v", err)
		}
		req, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		record := func(reason string) error {
			_, err := repo.UpdateBurgerStatRecalcFailure(ctx, burger, req.Version, domain.RecalcFailure{NextAttemptAt: time.Now(), Reason: reason})
			return err
		}

		if err := record(strings.Repeat("あ", domain.MaxRecalcFailureReasonChars)); err != nil {
			t.Errorf("上限ちょうどの理由が記録できない: %v", err)
		}
		if err := record(strings.Repeat("あ", domain.MaxRecalcFailureReasonChars+1)); err == nil {
			t.Error("上限を超える理由が記録できてしまった")
		}
	})
}
