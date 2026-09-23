package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// burgerQueryFake は、手書きの usecase.BurgerQuery の test double である。存在しない id は
// domain.ErrBurgerNotFound を返す（shop/review の Query fake と同じ規約）。
type burgerQueryFake struct {
	burgers map[string]domain.BurgerDetail
	shops   map[string][]domain.Shop
}

func newBurgerQueryFake() *burgerQueryFake {
	return &burgerQueryFake{burgers: map[string]domain.BurgerDetail{}, shops: map[string][]domain.Shop{}}
}

func (f *burgerQueryFake) GetBurgerWithStats(_ context.Context, id string) (domain.BurgerDetail, error) {
	if detail, ok := f.burgers[id]; ok {
		return detail, nil
	}
	return domain.BurgerDetail{}, domain.ErrBurgerNotFound
}

func (f *burgerQueryFake) ListBurgerShops(_ context.Context, burgerID string) ([]domain.Shop, error) {
	return f.shops[burgerID], nil
}

var _ usecase.BurgerQuery = (*burgerQueryFake)(nil)

// newBurgersRouter は、burger の handler だけを配線する（他の usecase は使わないので nil のまま）。
func newBurgersRouter(query *burgerQueryFake) http.Handler {
	return handler.NewRouter(okPinger, nil, unusedSignups(), nil, usecase.NewBurgers(query), nil, nil, nil, nil, nil)
}

// TestGetBurger は GET /burgers/{id} を扱う：レビューのある burger は、紐づくショップと統計つきの
// 200 を返す（AC1）。レビューが1件もない burger は、統計の3項目がすべて null になり、0件と
// 区別できる（AC2）。存在しない・UUID の正規形でない id は、同一の 404 になる（AC3）。
func TestGetBurger(t *testing.T) {
	burgerID := uid.N(1)
	shopID := uid.N(2)

	t.Run("レビューのある burger は、名前・紐づくショップ・統計つきの 200 を返す", func(t *testing.T) {
		reviewCount, avg, weighted := int64(3), 4.2, 4.0
		query := newBurgerQueryFake()
		query.burgers[burgerID] = domain.BurgerDetail{
			ID: burgerID, Name: "Cheese",
			ReviewCount: &reviewCount, AverageRating: &avg, WeightedScore: &weighted,
		}
		query.shops[burgerID] = []domain.Shop{{ID: shopID, Name: "Active Diner", Status: domain.ShopStatusActive}}
		router := newBurgersRouter(query)

		rec := do(router, http.MethodGet, "/burgers/"+burgerID, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":"` + burgerID + `","name":"Cheese","shops":[{"id":"` + shopID + `","name":"Active Diner"}],` +
			`"average_rating":4.2,"weighted_score":4,"review_count":3}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("レビューが1件もない burger は、統計の3項目がすべて null になる(0件と区別できる)", func(t *testing.T) {
		query := newBurgerQueryFake()
		query.burgers[burgerID] = domain.BurgerDetail{ID: burgerID, Name: "Veggie"}
		router := newBurgersRouter(query)

		rec := do(router, http.MethodGet, "/burgers/"+burgerID, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":"` + burgerID + `","name":"Veggie","shops":[],"average_rating":null,"weighted_score":null,"review_count":null}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("存在しない id と UUID の正規形でない id は、同一の 404 になる", func(t *testing.T) {
		router := newBurgersRouter(newBurgerQueryFake())
		const notFoundBody = `{"error":"Burger not found"}`
		for _, path := range []string{"/burgers/" + uid.N(999), "/burgers/abc", "/burgers/1", "/burgers/" + upperUUID} {
			rec := do(router, http.MethodGet, path, "", "")
			if rec.Code != http.StatusNotFound || rec.Body.String() != notFoundBody {
				t.Errorf("GET %s = %d %s, want 404 %s", path, rec.Code, rec.Body, notFoundBody)
			}
		}
	})
}
