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

// textLimitsResponse は、入力欄ごとの文字数の上限（Unicode のコードポイント数）である。
// frontend は、文字数のカウンターの表示にだけ使い、超えたかどうかの判定は backend の 422 に任せる。
type textLimitsResponse struct {
	ReviewCommentMaxChars  int `json:"review_comment_max_chars"`
	BurgerNameMaxChars     int `json:"burger_name_max_chars"`
	ShopNameMaxChars       int `json:"shop_name_max_chars"`
	UsernameMaxChars       int `json:"username_max_chars"`
	BioMaxChars            int `json:"bio_max_chars"`
	ModerationNoteMaxChars int `json:"moderation_note_max_chars"`
}

// passwordLimitsResponse は、パスワードの長さの範囲である。文字数ではなくバイト数（bcrypt の入力の
// 上限に合わせている）なので、日本語などの 1 文字は 3 バイトと数える。
type passwordLimitsResponse struct {
	MinBytes int `json:"min_bytes"`
	MaxBytes int `json:"max_bytes"`
}

type metaResponse struct {
	Rating   ratingRangeResponse    `json:"rating"`
	Photo    photoLimitsResponse    `json:"photo"`
	Text     textLimitsResponse     `json:"text"`
	Password passwordLimitsResponse `json:"password"`
}

// handleMeta は GET /meta を処理する：frontend が描画・送信前の処理に使う、backend のルールの値
// （rating の範囲、写真の保存の上限、文字数の上限、パスワードの長さ）を返す（200。認証不要）。
// ルールを持つのは domain だけで、frontend は定数を複製しない。
func handleMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", metaCacheControl)
	writeJSON(w, http.StatusOK, newMetaResponse())
}

// newMetaResponse は、GET /meta と MCP の get_meta が共有する、規則の値の JSON 形式である。
func newMetaResponse() metaResponse {
	return metaResponse{
		Rating: ratingRangeResponse{Min: domain.MinRating, Max: domain.MaxRating},
		Photo:  photoLimitsResponse{MaxEdge: photo.MaxEdge, MaxBytes: maxPhotoBytes},
		Text: textLimitsResponse{
			ReviewCommentMaxChars:  domain.MaxCommentChars,
			BurgerNameMaxChars:     domain.MaxBurgerNameChars,
			ShopNameMaxChars:       domain.MaxShopNameChars,
			UsernameMaxChars:       domain.MaxUsernameChars,
			BioMaxChars:            domain.MaxBioChars,
			ModerationNoteMaxChars: domain.MaxModerationNoteChars,
		},
		Password: passwordLimitsResponse{MinBytes: domain.MinPasswordBytes, MaxBytes: domain.MaxPasswordBytes},
	}
}
