package handler

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// shopNotFoundMessage は、存在しない shop と隠された shop（および UUID の
// 正規形でない id）に共通の 404 body であり、shop が存在するかどうかをレスポンスから
// 決して明かさないようにする。
const shopNotFoundMessage = "Shop not found"

// shopResponse は GET /shops のトップレベル配列の要素 1 つである
// （frontend の Shop、domains/shops/api/types.ts。ワイヤ上は snake_case）。
type shopResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// PhotoURL は、ショップの写真(そのショップで、写真つきで最も新しいレビューの写真)の公開 URL で、
	// 写真つきのレビューがないときは null。AverageRating は評価の平均(小数 1 桁)で、レビューがないときは
	// null。ReviewCount はレビューの件数。集計の意味は domain.ShopSummary が持ち、ここは写すだけである。
	PhotoURL      *string  `json:"photo_url"`
	AverageRating *float64 `json:"average_rating"`
	ReviewCount   int64    `json:"review_count"`
}

// newShopResponse は、一覧に出るショップを、集計つきで wire の形にする(GET /shops と MCP の list_shops が共有する)。
func newShopResponse(listing domain.ShopListing) shopResponse {
	return shopResponse{
		ID:            listing.ID,
		Name:          listing.Name,
		Status:        string(listing.Status),
		PhotoURL:      listing.Summary.PhotoURL,
		AverageRating: listing.Summary.AverageRating,
		ReviewCount:   listing.Summary.ReviewCount,
	}
}

// shopDetailResponse は GET /shops/{id} の body である
// （frontend の ShopDetail）。CanReview は、viewer がこの shop に review を投稿できるか
// （domain の reviewable ルール）で、匿名は false。frontend は、この値で「レビューを書く」
// ボタンを出し分ける。
type shopDetailResponse struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Status         string               `json:"status"`
	ModerationNote *string              `json:"moderation_note"`
	Creator        *userRefResponse     `json:"creator"`
	Reviews        []shopReviewResponse `json:"reviews"`
	CanReview      bool                 `json:"can_review"`
	// PhotoURL・AverageRating・ReviewCount は、shopResponse と同じ集計である。
	PhotoURL      *string  `json:"photo_url"`
	AverageRating *float64 `json:"average_rating"`
	ReviewCount   int64    `json:"review_count"`
}

type userRefResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type shopReviewResponse struct {
	ID        string  `json:"id"`
	Rating    int     `json:"rating"`
	Comment   *string `json:"comment"`
	CreatedAt string  `json:"created_at"`
	// PhotoURL は review の写真の公開 URL で、添付がない場合は null である。
	// GET /shops/{id} に埋め込まれる review では、現状は常に
	// null である。Shops usecase は意図的に写真の storage へ配線
	// されていない。
	PhotoURL *string               `json:"photo_url"`
	User     *userRefResponse      `json:"user"`
	Burger   *reviewBurgerResponse `json:"burger"`
}

type reviewBurgerResponse struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	AverageRating float64 `json:"average_rating"`
	ReviewCount   int64   `json:"review_count"`
	WeightedScore float64 `json:"weighted_score"`
	Confidence    float64 `json:"confidence"`
}

// viewerPtr は OptionalAuth が context に置いた viewer を、usecase の
// 省略可能な形（nil = 匿名）に変換する。
func viewerPtr(r *http.Request) *domain.User {
	if viewer, ok := ViewerFrom(r.Context()); ok {
		return &viewer
	}
	return nil
}

// integerPattern は、page / per_page が整数として正しい構文（符号は省略可、
// あとは ASCII の数字だけ）であることを判定する。桁数は問わない。
var integerPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)

// pageParams は一覧 endpoint の page / per_page の query parameter を整数として
// パースする。空の値と省略は 0（デフォルトへのフォールバックを示す usecase の
// マーカー）であり、エラーではない。桁あふれする整数を含め、`[+-]?[0-9]+` の形の
// 値が整数である。int の範囲を超える整数は、Atoi が返す clamp 済みの値
// （math.MaxInt / math.MinInt）をそのまま渡す（補正は usecase の clampPage が
// 行う）。形が合わない値があれば、不正な引数のメッセージ（両方不正なら page、
// per_page の順で両方）を並べた 422 を書き込み済みで false を返すので、呼び出し
// 側は何も書かずに return する。
func pageParams(w http.ResponseWriter, r *http.Request) (page, perPage int, ok bool) {
	var msgs []string
	parse := func(name, msg string) int {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			return 0
		}
		if !integerPattern.MatchString(raw) {
			msgs = append(msgs, msg)
			return 0
		}
		n, _ := strconv.Atoi(raw) // 構文は検証済みなので、エラーは範囲外だけである。そのとき Atoi は clamp 済みの値を返す
		return n
	}
	page = parse("page", "Page must be an integer")
	perPage = parse("per_page", "Per page must be an integer")
	if len(msgs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: msgs})
		return 0, 0, false
	}
	return page, perPage, true
}

// handleListShops は GET /shops を処理する：（存在する場合の）viewer から
// 見える shop のトップレベルの JSON 配列で、keyword で絞り込まれ、
// ページネーションされる。page / per_page が整数でなければ 422 である。
// 次のページの有無は、レスポンスヘッダー X-Has-More（true / false）で返す。
func handleListShops(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, perPage, ok := pageParams(w, r)
		if !ok {
			return
		}
		list, hasMore, err := shops.List(r.Context(), viewerPtr(r), r.URL.Query().Get("keyword"), page, perPage)
		if err != nil {
			log.Printf("shops: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]shopResponse, 0, len(list)) // nil ではない：[] として marshal される
		for _, shop := range list {
			resp = append(resp, newShopResponse(shop))
		}
		setHasMore(w, hasMore)
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleGetShop は GET /shops/{id} を処理する：creator と reviews を伴う
// shop の詳細、または未知、隠された、UUID の正規形でない id に対する統一された 404。
func handleGetShop(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := shopIDPathValue(w, r)
		if !ok {
			return
		}
		detail, err := shops.Get(r.Context(), viewerPtr(r), id)
		if err != nil {
			if errors.Is(err, domain.ErrShopNotFound) {
				writeError(w, http.StatusNotFound, shopNotFoundMessage)
				return
			}
			log.Printf("shops: get: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, newShopDetailResponse(detail))
	}
}

func newShopDetailResponse(detail domain.ShopDetail) shopDetailResponse {
	resp := shopDetailResponse{
		ID:             detail.ID,
		Name:           detail.Name,
		Status:         string(detail.Status),
		ModerationNote: detail.ModerationNote,
		Creator:        newUserRefResponse(detail.Creator),
		Reviews:        make([]shopReviewResponse, 0, len(detail.Reviews)),
		CanReview:      detail.CanReview,
		PhotoURL:       detail.Summary.PhotoURL,
		AverageRating:  detail.Summary.AverageRating,
		ReviewCount:    detail.Summary.ReviewCount,
	}
	for _, review := range detail.Reviews {
		item := shopReviewResponse{
			ID:        review.ID,
			Rating:    review.Rating,
			Comment:   review.Comment,
			CreatedAt: review.CreatedAt.UTC().Format(time.RFC3339),
			User:      newUserRefResponse(review.User),
		}
		if review.Burger != nil {
			item.Burger = &reviewBurgerResponse{
				ID:            review.Burger.ID,
				Name:          review.Burger.Name,
				AverageRating: review.Burger.AverageRating,
				ReviewCount:   review.Burger.ReviewCount,
				WeightedScore: review.Burger.WeightedScore,
				Confidence:    review.Burger.Confidence,
			}
		}
		resp.Reviews = append(resp.Reviews, item)
	}
	return resp
}

func newUserRefResponse(ref *domain.UserRef) *userRefResponse {
	if ref == nil {
		return nil
	}
	return &userRefResponse{ID: ref.ID, Username: ref.Username}
}
