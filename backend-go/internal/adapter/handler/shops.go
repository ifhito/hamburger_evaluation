package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// shopNotFoundMessage is the shared 404 body for missing and hidden shops
// (and non-numeric ids), so responses never reveal whether a shop exists.
const shopNotFoundMessage = "Shop not found"

// shopResponse is one element of the GET /shops top-level array
// (frontend Shop in domains/shops/api/types.ts, snake_case).
type shopResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// shopDetailResponse is the GET /shops/{id} body (frontend ShopDetail).
type shopDetailResponse struct {
	ID             int64                `json:"id"`
	Name           string               `json:"name"`
	Status         string               `json:"status"`
	ModerationNote *string              `json:"moderation_note"`
	Creator        *userRefResponse     `json:"creator"`
	Reviews        []shopReviewResponse `json:"reviews"`
}

type userRefResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type shopReviewResponse struct {
	ID        int64                 `json:"id"`
	Rating    int                   `json:"rating"`
	Comment   *string               `json:"comment"`
	CreatedAt string                `json:"created_at"`
	User      *userRefResponse      `json:"user"`
	Burger    *reviewBurgerResponse `json:"burger"`
}

type reviewBurgerResponse struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	AverageRating float64 `json:"average_rating"`
	ReviewCount   int64   `json:"review_count"`
	WeightedScore float64 `json:"weighted_score"`
	Confidence    float64 `json:"confidence"`
}

// viewerPtr converts the OptionalAuth context viewer into the usecase's
// optional form (nil = anonymous).
func viewerPtr(r *http.Request) *domain.User {
	if viewer, ok := ViewerFrom(r.Context()); ok {
		return &viewer
	}
	return nil
}

// queryInt parses the named query parameter as an int, returning 0 (the
// usecase's fall-back-to-default marker) when absent or not a number.
func queryInt(r *http.Request, name string) int {
	n, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return 0
	}
	return n
}

// handleListShops serves GET /shops: a top-level JSON array of the shops
// visible to the (optional) viewer, filtered by keyword and paginated.
func handleListShops(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := shops.List(r.Context(), viewerPtr(r), r.URL.Query().Get("keyword"),
			queryInt(r, "page"), queryInt(r, "per_page"))
		if err != nil {
			log.Printf("shops: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]shopResponse, 0, len(list)) // non-nil: marshals as []
		for _, shop := range list {
			resp = append(resp, shopResponse{ID: shop.ID, Name: shop.Name, Status: string(shop.Status)})
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleGetShop serves GET /shops/{id}: the shop detail with creator and
// reviews, or the uniform 404 for unknown, hidden, and non-numeric ids.
func handleGetShop(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusNotFound, shopNotFoundMessage)
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
