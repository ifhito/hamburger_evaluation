package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// burgerRankingResponse は GET /burgers のトップレベル配列の要素 1 つである(ワイヤ上は snake_case)。
type burgerRankingResponse struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Shop          shopRefResponse `json:"shop"`
	AverageRating float64         `json:"average_rating"`
	WeightedScore float64         `json:"weighted_score"`
	ReviewCount   int64           `json:"review_count"`
}

func newBurgerRankingResponse(r domain.BurgerRanking) burgerRankingResponse {
	return burgerRankingResponse{
		ID:            r.ID,
		Name:          r.Name,
		Shop:          shopRefResponse{ID: r.Shop.ID, Name: r.Shop.Name},
		AverageRating: r.AverageRating,
		WeightedScore: r.WeightedScore,
		ReviewCount:   r.ReviewCount,
	}
}

// handleListBurgers は GET /burgers を処理する: weighted_score の高い順のバーガーのトップレベルの
// JSON 配列。review が 1 件もないバーガーは含まない。認証不要(viewer に依存しない一覧)。
// page / per_page が整数でなければ 422。次のページの有無は、レスポンスヘッダー
// X-Has-More(true / false)で返す。
func handleListBurgers(burgers *usecase.Burgers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, perPage, ok := pageParams(w, r)
		if !ok {
			return
		}
		list, hasMore, err := burgers.List(r.Context(), page, perPage)
		if err != nil {
			log.Printf("burgers: list: %v", err)
			writeInternalError(w)
			return
		}
		resp := make([]burgerRankingResponse, 0, len(list))
		for _, ranking := range list {
			resp = append(resp, newBurgerRankingResponse(ranking))
		}
		setHasMore(w, hasMore)
		writeJSON(w, http.StatusOK, resp)
	}
}

// burgerDetailResponse は GET /burgers/{id} の body である。
type burgerDetailResponse struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Shops         []shopRefResponse `json:"shops"`
	AverageRating *float64          `json:"average_rating"`
	WeightedScore *float64          `json:"weighted_score"`
	ReviewCount   *int64            `json:"review_count"`
}

func newBurgerDetailResponse(detail domain.BurgerDetail) burgerDetailResponse {
	shops := make([]shopRefResponse, 0, len(detail.Shops)) // nil ではない：[] として marshal される
	for _, s := range detail.Shops {
		shops = append(shops, shopRefResponse{ID: s.ID, Name: s.Name})
	}
	return burgerDetailResponse{
		ID: detail.ID, Name: detail.Name, Shops: shops,
		AverageRating: detail.AverageRating, WeightedScore: detail.WeightedScore, ReviewCount: detail.ReviewCount,
	}
}

// burgerIDPathValue は {id} の path value を取り出す。false は統一された burger の 404 が
// 既に書き込まれたことを意味する(shop/review の id と同じ規約。UUID の正規形でない id は
// 存在しない burger と同一に見える)。
func burgerIDPathValue(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, r, http.StatusNotFound, msgBurgerNotFound)
		return "", false
	}
	return id, true
}

// handleGetBurger は GET /burgers/{id} を処理する：紐づくショップ(viewer に見えるものだけ)と統計つきの
// burger、または未知/UUID の正規形でない id に対する統一された 404。
func handleGetBurger(burgers *usecase.Burgers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := burgerIDPathValue(w, r)
		if !ok {
			return
		}
		detail, err := burgers.Get(r.Context(), viewerPtr(r), id)
		if err != nil {
			if errors.Is(err, domain.ErrBurgerNotFound) {
				writeError(w, r, http.StatusNotFound, msgBurgerNotFound)
				return
			}
			log.Printf("burgers: get: %v", err)
			writeInternalError(w)
			return
		}
		writeJSON(w, http.StatusOK, newBurgerDetailResponse(detail))
	}
}
