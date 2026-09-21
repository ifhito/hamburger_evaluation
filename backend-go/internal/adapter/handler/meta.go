package handler

import (
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// metaCacheControl は GET /meta のキャッシュの指定である。値はコードの定数で、デプロイで
// しか変わらないので、共有キャッシュにも置いてよい。
const metaCacheControl = "public, max-age=3600"

type ratingRangeResponse struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// photoLimitsResponse は、写真の保存の上限である。frontend は、これを超える写真を送る前に縮小する
// (縮小の判断に使う値は、backend だけが持つ)。
type photoLimitsResponse struct {
	MaxEdge  int   `json:"max_edge"`
	MaxBytes int64 `json:"max_bytes"`
}

type metaResponse struct {
	Rating ratingRangeResponse `json:"rating"`
	Photo  photoLimitsResponse `json:"photo"`
}

// handleMeta は GET /meta を処理する：frontend が描画・送信前の処理に使う、backend のルールの値
// （rating の範囲、写真の保存の上限）を返す（200。認証不要）。ルールを持つのは domain だけで、frontend は定数を複製しない。
func handleMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", metaCacheControl)
	writeJSON(w, http.StatusOK, metaResponse{
		Rating: ratingRangeResponse{Min: domain.MinRating, Max: domain.MaxRating},
		Photo:  photoLimitsResponse{MaxEdge: photo.MaxEdge, MaxBytes: maxPhotoBytes},
	})
}
