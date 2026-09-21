package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/statsworkertest"
)

// TestReviewWritesDeferStatsIntegration は、本物の PostgreSQL と router を通して、レビューの投稿・編集・
// 削除が、統計の計算を待たずに応答を返し、バーガーの統計は、ワーカーが動いたあとに表示へ反映される
// ことを確かめる(統計は、書き込みの少しあとに反映される。この性質を結果整合と呼ぶ)。
// バーガーの統計は、レビューに添えて表示されるので、確かめる間は、いつも 1 件以上のレビューが残るようにする。
func TestReviewWritesDeferStatsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)
	_, aliceAuth := signupUser(t, router, "alice", "alice@example.com", "Password123!")
	_, bobAuth := signupUser(t, router, "bob", "bob@example.com", "Password123!")

	var shopID, burgerID string
	if err := conn.QueryRow(ctx, `INSERT INTO shops (name, status) VALUES ('Active One', 1) RETURNING id`).Scan(&shopID); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	if err := conn.QueryRow(ctx, `INSERT INTO burgers (name) VALUES ('Cheese') RETURNING id`).Scan(&burgerID); err != nil {
		t.Fatalf("insert burger: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
		t.Fatalf("link burger: %v", err)
	}
	worker := statsworkertest.NewWorker(conn, infra.SystemClock{})

	// shownStats は、ショップの詳細のレビューに添えて表示される、バーガーの統計を返す。
	shownStats := func() (count int64, average float64) {
		t.Helper()
		rec := do(router, http.MethodGet, "/shops/"+shopID, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("shop detail status = %d (body %s)", rec.Code, rec.Body)
		}
		var body struct {
			Reviews []struct {
				Burger struct {
					ReviewCount   int64   `json:"review_count"`
					AverageRating float64 `json:"average_rating"`
				} `json:"burger"`
			} `json:"reviews"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Reviews) == 0 {
			t.Fatalf("ショップの詳細のレビューを読めない (%v): %s", err, rec.Body)
		}
		return body.Reviews[0].Burger.ReviewCount, body.Reviews[0].Burger.AverageRating
	}
	assertShown := func(step string, wantCount int64, wantAverage float64) {
		t.Helper()
		if count, average := shownStats(); count != wantCount || average != wantAverage {
			t.Errorf("%s: 表示される統計 = (件数 %d, 平均 %v), want (件数 %d, 平均 %v)", step, count, average, wantCount, wantAverage)
		}
	}
	postReview := func(auth string, rating int) string {
		t.Helper()
		body := fmt.Sprintf(`{"review":{"rating":%d,"comment":"tasty","shop_id":%q,"burger_id":%q}}`, rating, shopID, burgerID)
		rec := do(router, http.MethodPost, "/reviews", body, auth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /reviews = %d (body %s), want 201", rec.Code, rec.Body)
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode review: %v", err)
		}
		return created.ID
	}

	aliceReview := postReview(aliceAuth, 4)
	// 投稿の応答は返ったが、統計はまだ計算されず、再計算の依頼だけが登録されている。
	assertShown("1 件目の投稿の直後", 0, 0)
	if n := dbtest.CountRecalcRequests(ctx, t, conn, burgerID); n != 1 {
		t.Errorf("1 件目の投稿の直後の再計算の依頼 = %d 件, want 1 件", n)
	}
	statsworkertest.Settle(ctx, t, worker)
	assertShown("1 件目の投稿のあとにワーカーが動いた後", 1, 4)

	bobReview := postReview(bobAuth, 2)
	assertShown("2 件目の投稿の直後", 1, 4)
	statsworkertest.Settle(ctx, t, worker)
	assertShown("2 件目の投稿のあとにワーカーが動いた後", 2, 3)

	rec := do(router, http.MethodPut, "/reviews/"+bobReview, `{"review":{"rating":5,"comment":"changed my mind"}}`, bobAuth)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /reviews/%s = %d (body %s), want 200", bobReview, rec.Code, rec.Body)
	}
	assertShown("編集の直後", 2, 3)
	statsworkertest.Settle(ctx, t, worker)
	assertShown("編集のあとにワーカーが動いた後", 2, 4.5)

	rec = do(router, http.MethodDelete, "/reviews/"+aliceReview, "", aliceAuth)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /reviews/%s = %d (body %s), want 204", aliceReview, rec.Code, rec.Body)
	}
	assertShown("削除の直後", 2, 4.5)
	statsworkertest.Settle(ctx, t, worker)
	assertShown("削除のあとにワーカーが動いた後", 1, 5)
	if n := dbtest.CountRecalcRequests(ctx, t, conn, burgerID); n != 0 {
		t.Errorf("再計算のあとの依頼 = %d 件, want 0 件", n)
	}
}
