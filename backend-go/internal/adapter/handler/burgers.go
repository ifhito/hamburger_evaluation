package handler

import (
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
