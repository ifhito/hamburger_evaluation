package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/statsworkertest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
)

func jsonKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("JSON のオブジェクトではない: %s (%v)", raw, err)
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestShopSummaryFields は、ショップの一覧・詳細の応答に、写真の URL・平均評価・レビュー件数が付き、
// 既存の項目は変わらずに残ること(足しただけ)を、キーの集合で確かめる。
func TestShopSummaryFields(t *testing.T) {
	repo := seedShops(uid.N(1))
	repo.summaries = map[string]domain.ShopSummary{
		uid.N(1): domain.ShopSummary{ReviewCount: 3, AverageRating: shopPtr(4.3), PhotoKey: shopPtr("reviews/latest.jpg")},
	}
	router, aliceAuth, _, _ := newShopsRouter(t, repo)

	t.Run("一覧は、レビューのあるショップに写真の URL・平均評価(小数 1 桁)・件数を付け、レビューのないショップには null・null・0 を付ける", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/shops", "", aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		want := `[{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","photo_url":"/photos/reviews/latest.jpg","average_rating":4.3,"review_count":3},` +
			`{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","photo_url":null,"average_rating":null,"review_count":0}]`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("詳細も、同じ集計を付け、既存の項目はそのまま残る", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/shops/"+uid.N(1), "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		var body struct {
			PhotoURL      *string  `json:"photo_url"`
			AverageRating *float64 `json:"average_rating"`
			ReviewCount   int64    `json:"review_count"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.PhotoURL == nil || *body.PhotoURL != "/photos/reviews/latest.jpg" || body.AverageRating == nil || *body.AverageRating != 4.3 || body.ReviewCount != 3 {
			t.Errorf("集計 = %+v, want 写真の URL・4.3・3", body)
		}
		wantKeys := []string{"average_rating", "can_review", "creator", "id", "moderation_note", "name", "photo_url", "review_count", "reviews", "status"}
		if got := jsonKeys(t, rec.Body.Bytes()); !slices.Equal(got, wantKeys) {
			t.Errorf("詳細のキー = %v, want %v(既存のキー + 集計の 3 つ)", got, wantKeys)
		}
	})

	t.Run("レビューのないショップの詳細は、集計が null・null・0 になり、エラーにならない", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/shops/"+uid.N(2), "", aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		for _, want := range []string{`"photo_url":null`, `"average_rating":null`, `"review_count":0`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Errorf("body = %s, want it to contain %s", rec.Body, want)
			}
		}
	})

	t.Run("一覧の 1 件のキーは、既存の 3 つ + 集計の 3 つだけである", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/shops", "", "")
		var items []json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil || len(items) != 1 {
			t.Fatalf("body = %s (%v), want 1 item", rec.Body, err)
		}
		wantKeys := []string{"average_rating", "id", "name", "photo_url", "review_count", "status"}
		if got := jsonKeys(t, items[0]); !slices.Equal(got, wantKeys) {
			t.Errorf("一覧のキー = %v, want %v", got, wantKeys)
		}
	})
}

// TestMCPListShopsCarriesSummary は、MCP の list_shops が、REST の GET /shops と同じ集計(平均評価・件数・写真)を返すことを確かめる。
func TestMCPListShopsCarriesSummary(t *testing.T) {
	k := newMCPKit(t)
	k.shops.summaries = map[string]domain.ShopSummary{
		uid.N(1): domain.ShopSummary{ReviewCount: 3, AverageRating: shopPtr(4.3)},
	}
	alice := k.connect(t, k.token(k.alice, readScope))

	text, isErr := call(t, alice, "list_shops", nil)
	if isErr {
		t.Fatalf("list_shops failed: %s", text)
	}
	for _, want := range []string{
		`"name":"Active Diner","status":"active","photo_url":null,"average_rating":4.3,"review_count":3`,
		`"name":"Alice Pending","status":"pending","photo_url":null,"average_rating":null,"review_count":0`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("list_shops = %s, want it to contain %s", text, want)
		}
	}
}

// TestShopSummaryThroughRealQuery は、本物の DB・本物の ShopQuery(SQL・uuid[] の引数)・写真の URL の組み立て・
// handler をつないで、一覧と詳細の応答に、写真の URL・平均評価・件数が入ることを確かめる(fake を使わない通しの確認)。
func TestShopSummaryThroughRealQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	insertUser := `INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice")
	shop := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO shops (name, status) VALUES ('Real Diner', 1) RETURNING id`)
	quiet := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO shops (name, status) VALUES ('Quiet Diner', 1) RETURNING id`)
	burger := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ('Classic') RETURNING id`)
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shop, burger); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		rating    int
		photo     any
		discarded bool
		minute    int
	}{
		{5, "reviews/old.jpg", false, 1},
		{4, "reviews/latest.jpg", false, 2}, // 写真つきで最も新しい(削除されていない)
		{1, "reviews/discarded.jpg", true, 3},
	} {
		var discardedAt any
		if r.discarded {
			discardedAt = "2026-01-02T00:00:00Z"
		}
		if _, err := conn.Exec(ctx, `INSERT INTO reviews (rating, user_id, burger_id, photo_key, discarded_at, created_at)
			VALUES ($1, $2, $3, $4, $5, '2026-01-01T10:00:00Z'::timestamptz + make_interval(mins => $6))`,
			r.rating, alice, burger, r.photo, discardedAt, r.minute); err != nil {
			t.Fatal(err)
		}
	}

	users, auth, _ := newAuthKit()
	shopFake := &shopStoreFake{}
	shops := shopsUsecase(query.NewShopQuery(conn), shopFake)
	router := handler.NewRouter(okPinger, auth, unusedSignups(), shops, nil,
		reviewsUsecase(newReviewStoreFake(), storage.NewDisk(t.TempDir(), "/photos")), usersUsecase(users, hasherFake{}), nil, nil, nil)

	// 集計は、あとからワーカーが計算する(結果整合)。レビューを入れただけの時点では、まだ集計されていない。
	rec := do(router, http.MethodGet, "/shops", "", "")
	unstated := `[{"id":"` + quiet + `","name":"Quiet Diner","status":"active","photo_url":null,"average_rating":null,"review_count":0},` +
		`{"id":"` + shop + `","name":"Real Diner","status":"active","photo_url":null,"average_rating":null,"review_count":0}]`
	if rec.Code != http.StatusOK || rec.Body.String() != unstated {
		t.Errorf("集計の前の一覧 = %d %s, want %s(まだ集計されていないショップは、0・null・null)", rec.Code, rec.Body, unstated)
	}

	if _, err := conn.Exec(ctx, `INSERT INTO shop_stats_recalc_requests (shop_id) VALUES ($1)`, shop); err != nil {
		t.Fatal(err)
	}
	statsworkertest.SettleShops(ctx, t, statsworkertest.NewShopWorker(conn, uowtest.Clock{T: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}))

	rec = do(router, http.MethodGet, "/shops", "", "")
	want := `[{"id":"` + quiet + `","name":"Quiet Diner","status":"active","photo_url":null,"average_rating":null,"review_count":0},` +
		`{"id":"` + shop + `","name":"Real Diner","status":"active","photo_url":"/photos/reviews/latest.jpg","average_rating":4.5,"review_count":2}]`
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Errorf("集計のあとの一覧 = %d %s, want %s", rec.Code, rec.Body, want)
	}

	rec = do(router, http.MethodGet, "/shops/"+shop, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("詳細 = %d %s", rec.Code, rec.Body)
	}
	for _, want := range []string{`"photo_url":"/photos/reviews/latest.jpg"`, `"average_rating":4.5`, `"review_count":2`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("詳細 = %s, want it to contain %s", rec.Body, want)
		}
	}
}
