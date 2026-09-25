package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// burgerQueryFake は、手書きの usecase.BurgerQuery の test double である。存在しない id は
// domain.ErrBurgerNotFound を返す（shop/review の Query fake と同じ規約）。ランキング一覧
// (ListBurgerRankings)向けの rankings/hasMore/rankingErr/listCalls/lastLimit/lastOffset は、
// GET /burgers の応答の形と配線を検証するための記録・スタブである。
type burgerQueryFake struct {
	burgers map[string]domain.BurgerDetail
	shops   map[string][]domain.Shop

	rankings              []domain.BurgerRanking
	hasMore               bool
	rankingErr            error
	listCalls             int
	lastLimit, lastOffset int32
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

func (f *burgerQueryFake) ListBurgerRankings(_ context.Context, limit, offset int32) ([]domain.BurgerRanking, bool, error) {
	f.listCalls++
	f.lastLimit, f.lastOffset = limit, offset
	if f.rankingErr != nil {
		return nil, false, f.rankingErr
	}
	return f.rankings, f.hasMore, nil
}

var _ usecase.BurgerQuery = (*burgerQueryFake)(nil)

// newBurgersRouter は、burger の handler だけを配線する（他の usecase は使わないので nil のまま）。
func newBurgersRouter(query *burgerQueryFake) http.Handler {
	return handler.NewRouter(okPinger, nil, unusedSignups(), nil, usecase.NewBurgers(query, storage.NewDisk("", "/photos")), nil, nil, nil, nil, nil)
}

// TestGetBurger は GET /burgers/{id} を扱う：レビューのある burger は、紐づくショップと統計つきの
// 200 を返す。レビューが1件もない burger は、統計の3項目がすべて null になり、0件と
// 区別できる。存在しない・UUID の正規形でない id は、同一の 404 になる。
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
		want := `{"photo_url":null,"id":"` + burgerID + `","name":"Cheese","shops":[{"id":"` + shopID + `","name":"Active Diner"}],` +
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
		want := `{"photo_url":null,"id":"` + burgerID + `","name":"Veggie","shops":[],"average_rating":null,"weighted_score":null,"review_count":null}`
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

// TestListBurgers は HTTP レベルで、GET /burgers の応答の形と配線(usecase への引数の受け渡し)を
// 検証する。並び順・除外の正しさは internal/adapter/query の DB 統合テストが担う。
func TestListBurgers(t *testing.T) {
	t.Run("バーガーが 1 件もなければ、空配列(null ではない)を返す", func(t *testing.T) {
		fake := newBurgerQueryFake()
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); got != `[]` {
			t.Errorf("body = %s, want []", got)
		}
	})

	t.Run("各要素は id・name・shop・average_rating・weighted_score・review_count を持ち、X-Has-More は fake の値を反映する", func(t *testing.T) {
		fake := newBurgerQueryFake()
		fake.rankings = []domain.BurgerRanking{
			{
				ID:            "11111111-1111-1111-1111-111111111111",
				Name:          "Classic Burger",
				Shop:          domain.ShopRef{ID: "22222222-2222-2222-2222-222222222222", Name: "Diner"},
				AverageRating: 4.5,
				WeightedScore: 4.2,
				ReviewCount:   10,
			},
		}
		fake.hasMore = true
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		want := `[{"photo_url":null,"id":"11111111-1111-1111-1111-111111111111","name":"Classic Burger",` +
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
		fake := newBurgerQueryFake()
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if got := rec.Header().Get("X-Has-More"); got != "false" {
			t.Errorf("X-Has-More = %q, want false", got)
		}
	})

	t.Run("page が整数でなければ 422 で、usecase は呼ばれない", func(t *testing.T) {
		fake := newBurgerQueryFake()
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers?page=abc", "", "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body %s)", rec.Code, rec.Body)
		}
		if fake.listCalls != 0 {
			t.Errorf("ListBurgerRankings was called %d times, want 0 for a 422", fake.listCalls)
		}
	})

	t.Run("page / per_page から計算した limit / offset を usecase に渡す", func(t *testing.T) {
		fake := newBurgerQueryFake()
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers?page=3&per_page=7", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		if fake.listCalls != 1 {
			t.Fatalf("calls = %d, want 1", fake.listCalls)
		}
		if fake.lastLimit != 7 || fake.lastOffset != 14 {
			t.Errorf("limit/offset = %d/%d, want 7/14", fake.lastLimit, fake.lastOffset)
		}
	})

	t.Run("query の失敗は 500 として表面化する", func(t *testing.T) {
		fake := newBurgerQueryFake()
		fake.rankingErr = io.ErrUnexpectedEOF
		rec := do(newBurgersRouter(fake), http.MethodGet, "/burgers", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (body %s)", rec.Code, rec.Body)
		}
	})
}

func TestBurgerPhotoResponse(t *testing.T) {
	key := "reviews/example.jpg"
	for _, tt := range []struct {
		name   string
		status domain.ShopStatus
		key    *string
		want   string
	}{
		{"公開店舗の写真URLを返す", domain.ShopStatusActive, &key, `"/photos/reviews/example.jpg"`},
		{"写真がない場合はnull", domain.ShopStatusActive, nil, `null`},
		{"非公開店舗しかない場合はnull", domain.ShopStatusPending, &key, `null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := newBurgerQueryFake()
			q.burgers[uid.N(1)] = domain.BurgerDetail{ID: uid.N(1), Name: "バーガー", PhotoKey: tt.key}
			q.shops[uid.N(1)] = []domain.Shop{{ID: uid.N(2), Status: tt.status}}
			rec := do(newBurgersRouter(q), http.MethodGet, "/burgers/"+uid.N(1), "", "")
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || string(body["photo_url"]) != tt.want {
				t.Fatalf("body=%s", rec.Body)
			}
		})
	}
	q := newBurgerQueryFake()
	q.rankings = []domain.BurgerRanking{{ID: uid.N(1), PhotoKey: &key}}
	rec := do(newBurgersRouter(q), http.MethodGet, "/burgers", "", "")
	var body []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || string(body[0]["photo_url"]) != `"/photos/reviews/example.jpg"` {
		t.Fatalf("body=%s", rec.Body)
	}
}
