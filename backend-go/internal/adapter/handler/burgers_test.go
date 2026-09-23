package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// burgerQueryFake は、手書きの usecase.BurgerQuery の test double である。calls・lastLimit・
// lastOffset は、handler が usecase に渡した引数を検証するための記録である。
type burgerQueryFake struct {
	rankings              []domain.BurgerRanking
	hasMore               bool
	calls                 int
	lastLimit, lastOffset int32
}

var _ usecase.BurgerQuery = (*burgerQueryFake)(nil)

func (f *burgerQueryFake) ListBurgerRankings(_ context.Context, limit, offset int32) ([]domain.BurgerRanking, bool, error) {
	f.calls++
	f.lastLimit, f.lastOffset = limit, offset
	return f.rankings, f.hasMore, nil
}

// newBurgersRouter は、GET /burgers だけを検証するための最小の router を組み立てる。この endpoint は
// viewer に依存しないので、認証まわりの usecase はすべて未使用のまま nil で渡す。
func newBurgersRouter(fake *burgerQueryFake) http.Handler {
	return handler.NewRouter(okPinger, nil, unusedSignups(), nil, usecase.NewBurgers(fake), nil, nil, nil, nil, nil)
}

// TestListBurgers は HTTP レベルで、GET /burgers の応答の形と配線(usecase への引数の受け渡し)を
// 検証する。並び順・除外の正しさは internal/adapter/query の DB 統合テストが担う。
func TestListBurgers(t *testing.T) {
	t.Run("バーガーが 1 件もなければ、空配列(null ではない)を返す", func(t *testing.T) {
		fake := &burgerQueryFake{}
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); got != `[]` {
			t.Errorf("body = %s, want []", got)
		}
	})

	t.Run("各要素は id・name・shop・average_rating・weighted_score・review_count を持ち、X-Has-More は fake の値を反映する", func(t *testing.T) {
		fake := &burgerQueryFake{
			rankings: []domain.BurgerRanking{
				{
					ID:            "11111111-1111-1111-1111-111111111111",
					Name:          "Classic Burger",
					Shop:          domain.ShopRef{ID: "22222222-2222-2222-2222-222222222222", Name: "Diner"},
					AverageRating: 4.5,
					WeightedScore: 4.2,
					ReviewCount:   10,
				},
			},
			hasMore: true,
		}
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		want := `[{"id":"11111111-1111-1111-1111-111111111111","name":"Classic Burger",` +
			`"shop":{"id":"22222222-2222-2222-2222-222222222222","name":"Diner"},` +
			`"average_rating":4.5,"weighted_score":4.2,"review_count":10}]`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
		if got := rec.Header().Get("X-Has-More"); got != "true" {
			t.Errorf("X-Has-More = %q, want true", got)
		}
	})

	t.Run("hasMore が false なら X-Has-More も false になる", func(t *testing.T) {
		fake := &burgerQueryFake{}
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if got := rec.Header().Get("X-Has-More"); got != "false" {
			t.Errorf("X-Has-More = %q, want false", got)
		}
	})

	t.Run("page が整数でなければ 422 で、usecase は呼ばれない", func(t *testing.T) {
		fake := &burgerQueryFake{}
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers?page=abc", "", "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body %s)", rec.Code, rec.Body)
		}
		if fake.calls != 0 {
			t.Errorf("ListBurgerRankings was called %d times, want 0 for a 422", fake.calls)
		}
	})

	t.Run("page / per_page から計算した limit / offset を usecase に渡す", func(t *testing.T) {
		fake := &burgerQueryFake{}
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers?page=3&per_page=7", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		if fake.calls != 1 {
			t.Fatalf("calls = %d, want 1", fake.calls)
		}
		if fake.lastLimit != 7 || fake.lastOffset != 14 {
			t.Errorf("limit/offset = %d/%d, want 7/14", fake.lastLimit, fake.lastOffset)
		}
	})
}
