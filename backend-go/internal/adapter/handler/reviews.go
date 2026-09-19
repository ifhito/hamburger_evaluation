package handler

import (
	"errors"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
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
// and PUT /reviews/{id} (PUT ignores shop_id/burger_id/burger_name — a
// review never moves to another burger). On POST the burger is named by
// burger_id or, when that is absent, by burger_name (find-or-create, the
// frontend contract — S6 P3-1); precedence is the usecase's decision.
// Missing fields decode to zero values, which the domain rejects —
// Rails-parity 422 rather than 400.
type reviewParamsRequest struct {
	Review struct {
		Rating     int    `json:"rating"`
		Comment    string `json:"comment"`
		ShopID     int64  `json:"shop_id"`
		BurgerID   int64  `json:"burger_id"`
		BurgerName string `json:"burger_name"`
	} `json:"review"`
}

const (
	// maxPhotoBytes caps the raw photo upload at 5 MiB (S10 AC3); larger
	// uploads get 422, not 413 — the global review body cap is wider.
	maxPhotoBytes int64 = 5 << 20
	// maxMultipartTextBytes caps each text field of a multipart review
	// submission. Small next to the photo cap, yet roomy enough for any
	// realistic comment (the JSON path is capped only by the body limit).
	maxMultipartTextBytes int64 = 64 << 10
)

const photoTooLargeMessage = "Photo is too large (max 5MB)"

const photoUnsupportedMessage = "Photo must be a JPEG, PNG, or WebP image"

// multipartReviewForm carries the flat fields of a multipart/form-data
// review submission (S10 wire contract): the same values as
// reviewParamsRequest plus the processed photo (nil when the photo part
// is absent).
type multipartReviewForm struct {
	rating     int
	comment    string
	shopID     int64
	burgerID   int64
	burgerName string
	photo      *photo.Processed
}

// isMultipart reports whether the request declares multipart/form-data
// (the S10 photo submission path); everything else stays on the existing
// JSON path (backward compatibility).
func isMultipart(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "multipart/form-data"
}

// decodeReviewMultipart streams the multipart body via r.MultipartReader —
// the raw upload is never buffered wholesale. Text fields are read with a
// small per-field cap, unknown parts are ignored (NextPart discards their
// bodies), and the photo part is validated/normalized by photo.Process
// under the 5 MiB cap. false means the error response was already
// written: 413 when the global body cap tripped, 422 for an oversized or
// non-image photo, 400 for malformed multipart (including a duplicate
// photo part). Absent or non-numeric rating/shop_id/burger_id decode to 0
// and flow into the same validation/not-found paths as the JSON body.
func decodeReviewMultipart(w http.ResponseWriter, r *http.Request) (multipartReviewForm, bool) {
	var form multipartReviewForm
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return form, false
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return form, true
		}
		if err != nil {
			writeMultipartReadError(w, err)
			return form, false
		}
		name := part.FormName()
		if name == "photo" {
			if form.photo != nil {
				writeError(w, http.StatusBadRequest, "duplicate photo field")
				return form, false
			}
			processed, ok := readPhotoPart(w, part)
			if !ok {
				return form, false
			}
			form.photo = processed
			continue
		}
		value, ok := readTextPart(w, part)
		if !ok {
			return form, false
		}
		switch name {
		case "rating":
			form.rating, _ = strconv.Atoi(value)
		case "comment":
			form.comment = value
		case "shop_id":
			form.shopID, _ = strconv.ParseInt(value, 10, 64)
		case "burger_id":
			form.burgerID, _ = strconv.ParseInt(value, 10, 64)
		case "burger_name":
			form.burgerName = value
		}
		// Unknown fields are ignored, like decodeJSON's unknown-field
		// tolerance.
	}
}

// readTextPart reads one text field under the per-field cap; false means
// the error response was already written.
func readTextPart(w http.ResponseWriter, part *multipart.Part) (string, bool) {
	data, err := io.ReadAll(io.LimitReader(part, maxMultipartTextBytes+1))
	if err != nil {
		writeMultipartReadError(w, err)
		return "", false
	}
	if int64(len(data)) > maxMultipartTextBytes {
		writeError(w, http.StatusBadRequest, "multipart field too large")
		return "", false
	}
	return string(data), true
}

// readPhotoPart streams the photo part into photo.Process under the 5 MiB
// cap; the +1 sentinel byte detects "over the cap" without buffering the
// excess. The size check comes first so a huge non-image is reported as
// too large, never half-decoded. false means the error response was
// already written.
func readPhotoPart(w http.ResponseWriter, part *multipart.Part) (*photo.Processed, bool) {
	limited := &io.LimitedReader{R: part, N: maxPhotoBytes + 1}
	processed, err := photo.Process(limited)
	if limited.N == 0 { // the part held more than maxPhotoBytes
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{photoTooLargeMessage}})
		return nil, false
	}
	if err != nil {
		if errors.Is(err, photo.ErrUnsupportedImage) {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{photoUnsupportedMessage}})
			return nil, false
		}
		// Non-sentinel Process errors on this path are mid-stream read
		// failures, i.e. a broken multipart body (or the global body cap).
		writeMultipartReadError(w, err)
		return nil, false
	}
	return &processed, true
}

// writeMultipartReadError maps a mid-stream multipart read failure: the
// global body-cap MaxBytesReader surfaces as 413, anything else as a
// malformed body (400).
func writeMultipartReadError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid multipart body")
}

// newReviewResponse maps the domain payload onto the wire shape shared
// with shop detail reviews (frontend Review, snake_case).
func newReviewResponse(detail domain.ReviewDetail) shopReviewResponse {
	resp := shopReviewResponse{
		ID:        detail.ID,
		Rating:    detail.Rating,
		Comment:   detail.Comment,
		CreatedAt: detail.CreatedAt.UTC().Format(time.RFC3339),
		PhotoURL:  detail.PhotoURL,
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

// reviewListFilter parses the optional rating/keyword/shop_id query
// filters of GET /reviews (Rails ReviewQuery). An empty value counts as
// absent (params[:x].present?); false means the 422 for a non-integer
// rating or shop_id was already written — a deliberate fail-loud
// divergence from Rails, which casts garbage to 0 and silently returns an
// empty list.
func reviewListFilter(w http.ResponseWriter, r *http.Request) (usecase.ReviewListFilter, bool) {
	filter := usecase.ReviewListFilter{Keyword: r.URL.Query().Get("keyword")}
	if raw := r.URL.Query().Get("rating"); raw != "" {
		rating, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{"Rating must be an integer"}})
			return usecase.ReviewListFilter{}, false
		}
		filter.Rating = &rating
	}
	if raw := r.URL.Query().Get("shop_id"); raw != "" {
		shopID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{"Shop id must be an integer"}})
			return usecase.ReviewListFilter{}, false
		}
		filter.ShopID = &shopID
	}
	return filter, true
}

// handleListReviews serves GET /reviews: the public top-level JSON array
// of reviews of active shops' burgers, optionally narrowed by the
// rating/keyword/shop_id filters, newest first, paginated. The
// OptionalAuth viewer plays no filtering role here.
func handleListReviews(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, ok := reviewListFilter(w, r)
		if !ok {
			return
		}
		list, err := reviews.List(r.Context(), filter, queryInt(r, "page"), queryInt(r, "per_page"))
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
		var form multipartReviewForm
		if isMultipart(r) {
			var ok bool
			if form, ok = decodeReviewMultipart(w, r); !ok {
				return
			}
		} else {
			var req reviewParamsRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			form = multipartReviewForm{
				rating:     req.Review.Rating,
				comment:    req.Review.Comment,
				shopID:     req.Review.ShopID,
				burgerID:   req.Review.BurgerID,
				burgerName: req.Review.BurgerName,
			}
		}
		detail, err := reviews.Create(r.Context(), viewer, form.shopID, form.burgerID,
			form.burgerName, form.rating, form.comment, form.photo)
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
		// PUT uses only rating, comment, and photo; the multipart parser's
		// other fields are ignored, exactly like the JSON body's
		// shop_id/burger_id/burger_name.
		var form multipartReviewForm
		if isMultipart(r) {
			var ok bool
			if form, ok = decodeReviewMultipart(w, r); !ok {
				return
			}
		} else {
			var req reviewParamsRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			form = multipartReviewForm{rating: req.Review.Rating, comment: req.Review.Comment}
		}
		detail, err := reviews.Update(r.Context(), viewer, id, form.rating, form.comment, form.photo)
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
