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
	CityMaxChars           int `json:"city_max_chars"`
	StreetAddressMaxChars  int `json:"street_address_max_chars"`
}

// prefectureResponse は、都道府県の表の 1 件である。frontend は、ショップの住所の入力・表示・絞り込みに、
// この表を使う(都道府県の名前とコードを frontend に持たない)。
type prefectureResponse struct {
	Code   int    `json:"code"`
	NameJA string `json:"name_ja"`
	NameEN string `json:"name_en"`
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
	// Prefectures は、47 都道府県をコードの昇順で並べた表である。
	Prefectures []prefectureResponse `json:"prefectures"`
}

// handleMeta は GET /meta を処理する：frontend が描画・送信前の処理に使う、backend のルールの値
// （rating の範囲、写真の保存の上限、文字数の上限、パスワードの長さ、都道府県の表）を返す（200。認証不要）。
// ルールを持つのは domain だけで、frontend は定数を複製しない。
func handleMeta(loginProviders []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", metaCacheControl)
		writeJSON(w, http.StatusOK, httpMetaResponse{metaResponse: newMetaResponse(), LoginProviders: loginProviders})
	}
}

// httpMetaResponse は、GET /meta の応答である。規則の値(metaResponse)に、この環境で使えるサインイン方法を
// 足したもの。サインイン方法は、規則ではなく設定(環境変数)で決まるので、MCP の get_meta とは共有しない。
type httpMetaResponse struct {
	metaResponse
	// LoginProviders は、パスワードのほかに使えるサインイン方法の名前である(例: ["google"])。なければ空の配列。
	// frontend は、これに含まれるものだけ、サインインの画面にボタンを出す。
	LoginProviders []string `json:"login_providers"`
}

// newMetaResponse は、GET /meta と MCP の get_meta が共有する、規則の値の JSON 形式である。
func newMetaResponse() metaResponse {
	return metaResponse{
		Rating: ratingRangeResponse{Min: domain.MinRating, Max: domain.MaxRating},
		Photo:  photoLimitsResponse{MaxEdge: domain.MaxPhotoEdge, MaxBytes: domain.MaxPhotoBytes},
		Text: textLimitsResponse{
			ReviewCommentMaxChars:  domain.MaxCommentChars,
			BurgerNameMaxChars:     domain.MaxBurgerNameChars,
			ShopNameMaxChars:       domain.MaxShopNameChars,
			UsernameMaxChars:       domain.MaxUsernameChars,
			BioMaxChars:            domain.MaxBioChars,
			ModerationNoteMaxChars: domain.MaxModerationNoteChars,
			CityMaxChars:           domain.MaxCityChars,
			StreetAddressMaxChars:  domain.MaxStreetAddressChars,
		},
		Password:    passwordLimitsResponse{MinBytes: domain.MinPasswordBytes, MaxBytes: domain.MaxPasswordBytes},
		Prefectures: newPrefecturesResponse(),
	}
}

// prefectureNamesEN は、都道府県の英語の名前(表示用の翻訳)である。添字 + 1 がコードで、domain.Prefectures と
// 同じ 47 件・同じ順でなければならない(食い違いは TestPrefectureNamesENMatchDomain が検出する)。
var prefectureNamesEN = [...]string{
	"Hokkaido", "Aomori", "Iwate", "Miyagi", "Akita", "Yamagata", "Fukushima", "Ibaraki",
	"Tochigi", "Gunma", "Saitama", "Chiba", "Tokyo", "Kanagawa", "Niigata", "Toyama",
	"Ishikawa", "Fukui", "Yamanashi", "Nagano", "Gifu", "Shizuoka", "Aichi", "Mie",
	"Shiga", "Kyoto", "Osaka", "Hyogo", "Nara", "Wakayama", "Tottori", "Shimane",
	"Okayama", "Hiroshima", "Yamaguchi", "Tokushima", "Kagawa", "Ehime", "Kochi", "Fukuoka",
	"Saga", "Nagasaki", "Kumamoto", "Oita", "Miyazaki", "Kagoshima", "Okinawa",
}

func newPrefecturesResponse() []prefectureResponse {
	prefectures := domain.Prefectures()
	resp := make([]prefectureResponse, 0, len(prefectures))
	for _, p := range prefectures {
		resp = append(resp, prefectureResponse{Code: p.Code(), NameJA: p.Name(), NameEN: prefectureNamesEN[p.Code()-1]})
	}
	return resp
}
