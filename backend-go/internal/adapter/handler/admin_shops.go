package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// forbiddenMessage is the Rails-parity 403 body for authenticated viewers
// without moderation rights.
const forbiddenMessage = "Forbidden"

// shopParamsRequest is the {"shop":{"name":...}} wrapper of POST /shops
// and PUT /admin/shops/{id}. A missing wrapper or name decodes to "",
// which the domain rejects as blank — Rails-parity 422 rather than 400.
type shopParamsRequest struct {
	Shop struct {
		Name string `json:"name"`
	} `json:"shop"`
}

// rejectShopRequest is the POST /admin/shops/{id}/reject body. The note is
// a top-level key (not nested under "shop") and optional; the whole body
// may be absent.
type rejectShopRequest struct {
	ModerationNote *string `json:"moderation_note"`
}

// adminShopResponse is the shop submission/moderation payload (Rails
// ShopSerializer, frontend AdminShop): the shop with moderation note and
// creator, without reviews.
type adminShopResponse struct {
	ID             int64            `json:"id"`
	Name           string           `json:"name"`
	Status         string           `json:"status"`
	ModerationNote *string          `json:"moderation_note"`
	Creator        *userRefResponse `json:"creator"`
}

func newAdminShopResponse(detail domain.ShopDetail) adminShopResponse {
	return adminShopResponse{
		ID:             detail.ID,
		Name:           detail.Name,
		Status:         string(detail.Status),
		ModerationNote: detail.ModerationNote,
		Creator:        newUserRefResponse(detail.Creator),
	}
}

// requireViewer returns the viewer RequireAuth stored in the context. The
// false branch is a wiring bug (route registered without RequireAuth); it
// answers 500 so the bug cannot silently act as an anonymous request.
func requireViewer(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	viewer, ok := ViewerFrom(r.Context())
	if !ok {
		log.Printf("handler: %s %s: no viewer in context (route missing RequireAuth?)", r.Method, r.URL.Path)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
	return viewer, ok
}

// shopIDPathValue parses the {id} path value; false means the uniform
// shop 404 was already written (non-numeric ids look exactly like missing
// shops, the existing GET /shops/{id} convention).
func shopIDPathValue(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, shopNotFoundMessage)
		return 0, false
	}
	return id, true
}

// writeShopModerationError maps the shop submission/moderation usecase
// errors onto HTTP: authorization (decided in the usecase, never here) to
// 403, missing shops to the uniform 404, validation to 422, anything else
// to 500.
func writeShopModerationError(w http.ResponseWriter, op string, err error) {
	var vErr *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, domain.ErrShopNotFound):
		writeError(w, http.StatusNotFound, shopNotFoundMessage)
	case errors.As(err, &vErr):
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: vErr.Messages})
	default:
		log.Printf("shops: %s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// handleCreateShop serves POST /shops behind RequireAuth: 201 with the
// created (pending) shop and its creator, 422 on a blank name.
func handleCreateShop(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		var req shopParamsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		detail, err := shops.Create(r.Context(), viewer, req.Shop.Name)
		if err != nil {
			writeShopModerationError(w, "create", err)
			return
		}
		writeJSON(w, http.StatusCreated, newAdminShopResponse(detail))
	}
}

// handleAdminListShops serves GET /admin/shops: a top-level array of every
// shop, newest first, optionally filtered by ?status=.
func handleAdminListShops(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		list, err := shops.AdminList(r.Context(), viewer, r.URL.Query().Get("status"))
		if err != nil {
			writeShopModerationError(w, "admin list", err)
			return
		}
		resp := make([]adminShopResponse, 0, len(list)) // non-nil: marshals as []
		for _, detail := range list {
			resp = append(resp, newAdminShopResponse(detail))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleAdminUpdateShop serves PUT /admin/shops/{id}: renames the shop
// (name is the only editable attribute, Rails parity).
func handleAdminUpdateShop(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := shopIDPathValue(w, r)
		if !ok {
			return
		}
		var req shopParamsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		detail, err := shops.AdminUpdateName(r.Context(), viewer, id, req.Shop.Name)
		if err != nil {
			writeShopModerationError(w, "admin update", err)
			return
		}
		writeJSON(w, http.StatusOK, newAdminShopResponse(detail))
	}
}

// handleApproveShop serves POST /admin/shops/{id}/approve. Any request
// body is deliberately ignored — the transition takes no input.
func handleApproveShop(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := shopIDPathValue(w, r)
		if !ok {
			return
		}
		detail, err := shops.Approve(r.Context(), viewer, id)
		if err != nil {
			writeShopModerationError(w, "approve", err)
			return
		}
		writeJSON(w, http.StatusOK, newAdminShopResponse(detail))
	}
}

// handleRejectShop serves POST /admin/shops/{id}/reject. The body (and
// its moderation_note) is optional: an empty body rejects with a null
// note.
func handleRejectShop(shops *usecase.Shops) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := shopIDPathValue(w, r)
		if !ok {
			return
		}
		var req rejectShopRequest
		if !decodeOptionalJSON(w, r, &req) {
			return
		}
		detail, err := shops.Reject(r.Context(), viewer, id, req.ModerationNote)
		if err != nil {
			writeShopModerationError(w, "reject", err)
			return
		}
		writeJSON(w, http.StatusOK, newAdminShopResponse(detail))
	}
}
