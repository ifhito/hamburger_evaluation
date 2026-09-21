package query_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestBurgerStatsQueryListDueRecalcRequests は、再計算の依頼のうち「今、取り出してよいもの」を選ぶ
// 条件と並び順を、実際の PostgreSQL に対して検証する。データは SQL の INSERT で用意し、
// 書き込みの adapter/repository には依存しない。TEST_DATABASE_URL がなければスキップする。
func TestBurgerStatsQueryListDueRecalcRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	const maxAttempts = 3

	type seed struct {
		name     string
		attempts int
		next     *time.Time
	}
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }

	// 依頼を登録して、burger の id を名前から引ける形で返す。
	setup := func(t *testing.T, seeds ...seed) (*query.BurgerStatsQuery, map[string]string, func(ids ...string) []string) {
		t.Helper()
		conn, _ := dbtest.New(t)
		ids := map[string]string{}
		byID := map[string]string{}
		for _, s := range seeds {
			id := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, s.name)
			ids[s.name] = id
			byID[id] = s.name
			if _, err := conn.Exec(ctx,
				`INSERT INTO burger_stats_dirty (burger_id, attempts, next_attempt_at) VALUES ($1, $2, $3)`,
				id, s.attempts, s.next); err != nil {
				t.Fatalf("依頼の登録 %s: %v", s.name, err)
			}
		}
		names := func(ids ...string) []string {
			out := make([]string, 0, len(ids))
			for _, id := range ids {
				out = append(out, byID[id])
			}
			return out
		}
		return query.NewBurgerStatsQuery(conn), ids, names
	}
	burgerIDs := func(reqs []domain.RecalcRequest) []string {
		out := make([]string, 0, len(reqs))
		for _, r := range reqs {
			out = append(out, r.BurgerID)
		}
		return out
	}

	t.Run("時期が来ている依頼だけが返る(すぐのもの・時刻が過ぎたもの・ちょうどの時刻)", func(t *testing.T) {
		q, _, names := setup(t,
			seed{"immediate", 0, nil},
			seed{"past", 1, at(-time.Second)},
			seed{"exactly-now", 1, at(0)},
			seed{"future", 1, at(time.Second)},
		)
		reqs, err := q.ListDueRecalcRequests(ctx, now, maxAttempts, 10)
		if err != nil {
			t.Fatalf("ListDueRecalcRequests: %v", err)
		}
		got := names(burgerIDs(reqs)...)
		// 並び順は、時期の古い順で、すぐのものが先。
		want := []string{"immediate", "past", "exactly-now"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("取り出した依頼 = %v, want %v", got, want)
		}
	})

	t.Run("失敗の回数が上限に達した依頼は、時期が来ていても返らない", func(t *testing.T) {
		q, _, names := setup(t,
			seed{"retrying", maxAttempts - 1, at(-time.Second)},
			seed{"gave-up", maxAttempts, at(-time.Second)},
			seed{"gave-up-and-immediate", maxAttempts + 1, nil},
		)
		reqs, err := q.ListDueRecalcRequests(ctx, now, maxAttempts, 10)
		if err != nil {
			t.Fatalf("ListDueRecalcRequests: %v", err)
		}
		if got, want := names(burgerIDs(reqs)...), []string{"retrying"}; !reflect.DeepEqual(got, want) {
			t.Errorf("取り出した依頼 = %v, want %v", got, want)
		}
	})

	t.Run("取り出す件数は上限で切られ、残りは次に回る", func(t *testing.T) {
		q, _, names := setup(t,
			seed{"c", 1, at(-time.Minute)},
			seed{"a", 0, nil},
			seed{"b", 1, at(-2 * time.Minute)},
		)
		reqs, err := q.ListDueRecalcRequests(ctx, now, maxAttempts, 2)
		if err != nil {
			t.Fatalf("ListDueRecalcRequests: %v", err)
		}
		if got, want := names(burgerIDs(reqs)...), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
			t.Errorf("取り出した依頼 = %v, want すぐのものと、最も古いもの %v", got, want)
		}
	})

	t.Run("依頼の version と失敗の回数が、そのまま返る", func(t *testing.T) {
		q, ids, _ := setup(t, seed{"failing", 2, at(-time.Second)})
		reqs, err := q.ListDueRecalcRequests(ctx, now, maxAttempts, 10)
		if err != nil || len(reqs) != 1 {
			t.Fatalf("ListDueRecalcRequests = (%v, %v), want 1 件", reqs, err)
		}
		got := reqs[0]
		if got.BurgerID != ids["failing"] || got.Attempts != 2 || got.Version <= 0 {
			t.Errorf("依頼 = %+v, want burger=%s attempts=2 version>0", got, ids["failing"])
		}
	})

	t.Run("依頼がなければ、空の一覧が返る", func(t *testing.T) {
		q, _, _ := setup(t)
		reqs, err := q.ListDueRecalcRequests(ctx, now, maxAttempts, 10)
		if err != nil || len(reqs) != 0 {
			t.Errorf("ListDueRecalcRequests = (%v, %v), want 空の一覧", reqs, err)
		}
	})
}
