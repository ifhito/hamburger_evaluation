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

// reviewNotFoundMessage is the shared 404 body for missing, discarded,
// and non-numeric review ids, so soft-deleted reviews are
// indistinguishable from never-existing ones.
const reviewNotFoundMessage = "Review not found"

// burgerNotFoundMessage is the 404 body for a burger that does not exist
// or is not served by the requested shop.
const burgerNotFoundMessage = "Burger not found"

// reviewParamsRequest is the {"review":{...}} wrapper of POST /reviews
// and PUT /reviews/{id} (PUT ignores shop_id/burger_id — a review never
// moves to another burger). Missing fields decode to zero values, which
// the domain rejects — Rails-parity 422 rather than 400.
type reviewParamsRequest struct {
	Review struct {
		Rating   int    `json:"rating"`
		Comment  string `json:"comment"`
		ShopID   int64  `json:"shop_id"`
		BurgerID int64  `json:"burger_id"`
	} `json:"review"`
}

// newReviewResponse maps the domain payload onto the wire shape shared
// with shop detail reviews (frontend Review, snake_case).
func newReviewResponse(detail domain.ReviewDetail) shopReviewResponse {
	resp := shopReviewResponse{
		ID:        detail.ID,
		Rating:    detail.Rating,
		Comment:   detail.Comment,
		CreatedAt: detail.CreatedAt.UTC().Format(time.RFC3339),
		User:      newUserRefResponse(detail.User),
	}
	if detail.Burger != nil {
		resp.Burger = &reviewBurgerResponse{
			ID:            detail.Burger.ID,
			Name:          detail.Burger.Name,
			AverageRating: detail.Burger.AverageRating,
			ReviewCount:   detail.Burger.ReviewCount,
			WeightedScore: detail.Burger.WeightedScore,
			Confidence:    detail.Burger.Confidence,
		}
	}
	return resp
}

// reviewIDPathValue parses the {id} path value; false means the uniform
// review 404 was already written (non-numeric ids look exactly like
// missing reviews, the shop-id convention).
func reviewIDPathValue(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, reviewNotFoundMessage)
		return 0, false
	}
	return id, true
}

// writeReviewError maps the review usecase errors onto HTTP: the domain
// authorization decisions to 403, the three not-found sentinels to their
// endpoint-specific 404 bodies, validation to 422, anything else to 500.
func writeReviewError(w http.ResponseWriter, op string, err error) {
	var vErr *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, domain.ErrReviewNotFound):
		writeError(w, http.StatusNotFound, reviewNotFoundMessage)
	case errors.Is(err, domain.ErrShopNotFound):
		writeError(w, http.StatusNotFound, shopNotFoundMessage)
	case errors.Is(err, domain.ErrBurgerNotFound):
		writeError(w, http.StatusNotFound, burgerNotFoundMessage)
	case errors.As(err, &vErr):
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: vErr.Messages})
	default:
		log.Printf("reviews: %s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// handleListReviews serves GET /reviews: the public top-level JSON array
// of reviews of active shops' burgers, newest first, paginated. The
// OptionalAuth viewer plays no filtering role here.
func handleListReviews(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := reviews.List(r.Context(), queryInt(r, "page"), queryInt(r, "per_page"))
		if err != nil {
			log.Printf("reviews: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]shopReviewResponse, 0, len(list)) // non-nil: marshals as []
		for _, detail := range list {
			resp = append(resp, newReviewResponse(detail))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleGetReview serves GET /reviews/{id}: the review with author,
// burger, and stats, or the uniform 404 for unknown, discarded, and
// non-numeric ids.
func handleGetReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := reviewIDPathValue(w, r)
		if !ok {
			return
		}
		detail, err := reviews.Get(r.Context(), id)
		if err != nil {
			writeReviewError(w, "get", err)
			return
		}
		writeJSON(w, http.StatusOK, newReviewResponse(detail))
	}
}

// handleCreateReview serves POST /reviews behind RequireAuth: 201 with
// the created review. The check order (shop 404, reviewable 403, burger
// 404, validation 422) is decided in the usecase, never here.
func handleCreateReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		var req reviewParamsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		detail, err := reviews.Create(r.Context(), viewer, req.Review.ShopID, req.Review.BurgerID,
			req.Review.Rating, req.Review.Comment)
		if err != nil {
			writeReviewError(w, "create", err)
			return
		}
		writeJSON(w, http.StatusCreated, newReviewResponse(detail))
	}
}

// handleUpdateReview serves PUT /reviews/{id} behind RequireAuth: 200
// with the full payload. Ownership (author-only, no admin pass) is the
// domain's decision surfaced as 403.
func handleUpdateReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := reviewIDPathValue(w, r)
		if !ok {
			return
		}
		var req reviewParamsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		detail, err := reviews.Update(r.Context(), viewer, id, req.Review.Rating, req.Review.Comment)
		if err != nil {
			writeReviewError(w, "update", err)
			return
		}
		writeJSON(w, http.StatusOK, newReviewResponse(detail))
	}
}

// handleDeleteReview serves DELETE /reviews/{id} behind RequireAuth: a
// soft delete answered with 204 and no body.
func handleDeleteReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := reviewIDPathValue(w, r)
		if !ok {
			return
		}
		if err := reviews.Delete(r.Context(), viewer, id); err != nil {
			writeReviewError(w, "delete", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
