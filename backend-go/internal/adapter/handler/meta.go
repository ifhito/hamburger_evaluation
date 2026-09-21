package handler

import (
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// metaCacheControl は GET /meta のキャッシュの指定である。値はコードの定数で、デプロイで
// しか変わらないので、共有キャッシュにも置いてよい。
const metaCacheControl = "public, max-age=3600"

type ratingRangeResponse struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type metaResponse struct {
	Rating ratingRangeResponse `json:"rating"`
}

// handleMeta は GET /meta を処理する：frontend が描画に使う、domain のルールの値（今は rating の
// 範囲）を返す（200。認証不要）。ルールを持つのは domain だけで、frontend は定数を複製しない。
func handleMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", metaCacheControl)
	writeJSON(w, http.StatusOK, metaResponse{
		Rating: ratingRangeResponse{Min: domain.MinRating, Max: domain.MaxRating},
	})
}
