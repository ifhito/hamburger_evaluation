package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// forbiddenMessage は、moderation の権限を持たない認証済みの viewer に返す
// Rails-parity の 403 body である。
const forbiddenMessage = "Forbidden"

// shopParamsRequest は POST /shops と PUT /admin/shops/{id} の
// {"shop":{"name":...}} ラッパーである。wrapper や name が欠けている場合は
// "" にデコードされ、domain はそれを blank として拒否する。400 ではなく
// Rails-parity の 422 になる。
type shopParamsRequest struct {
	Shop struct {
		Name string `json:"name"`
	} `json:"shop"`
}

// rejectShopRequest は POST /admin/shops/{id}/reject の body である。
// note はトップレベルのキー（"shop" の下にネストされない）で任意であり、
// body 全体が存在しなくてもよい。
type rejectShopRequest struct {
	ModerationNote *string `json:"moderation_note"`
}

// adminShopResponse は shop の投稿/moderation の payload である。id、name、
// status、moderation_note、creator を持ち、reviews は持たない。
type adminShopResponse struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Status         string           `json:"status"`
	ModerationNote *string          `json:"moderation_note"`
	Creator        *userRefResponse `json:"creator"`
	// CanApprove・CanReject は、承認・却下の操作を画面が提示してよいか（domain が判断する）。
	CanApprove bool `json:"can_approve"`
	CanReject  bool `json:"can_reject"`
}

func newAdminShopResponse(detail domain.ShopDetail) adminShopResponse {
	return adminShopResponse{
		ID:             detail.ID,
		Name:           detail.Name,
		Status:         string(detail.Status),
		ModerationNote: detail.ModerationNote,
		Creator:        newUserRefResponse(detail.Creator),
		CanApprove:     detail.Shop.CanBeApproved(),
		CanReject:      detail.Shop.CanBeRejected(),
	}
}

// requireViewer は RequireAuth が context に格納した viewer を返す。false の
// 分岐は配線のバグ（RequireAuth なしで登録された route）を表し、500 を返す
// ことで、そのバグが匿名リクエストとして黙って通用しないようにする。
func requireViewer(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	viewer, ok := ViewerFrom(r.Context())
	if !ok {
		log.Printf("handler: %s %s: no viewer in context (route missing RequireAuth?)", r.Method, r.URL.Path)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
	return viewer, ok
}

// shopIDPathValue は {id} の path value を取り出す。false は統一された
// shop の 404 が既に書き込まれたことを意味する（UUID の正規形でない id は
// 存在しない shop とまったく同じに見える。既存の GET /shops/{id} の規約）。
// 形式の判定は domain.IsUUID が持つ。
func shopIDPathValue(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, http.StatusNotFound, shopNotFoundMessage)
		return "", false
	}
	return id, true
}

// writeShopModerationError は、shop の投稿/moderation の usecase のエラーを
// HTTP に対応させる：認可（usecase で決定し、ここでは決して決めない）は
// 403、存在しない shop は統一された 404、validation は 422、それ以外は
// 500 である。
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

// handleCreateShop は RequireAuth の背後で POST /shops を処理する：作成された
// （pending の）shop とその creator を伴う 201、name が blank の場合は 422。
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

// handleAdminListShops は GET /admin/shops を処理する：すべての shop の
// トップレベルの配列で、新しい順に並び、任意で ?status= により絞り込まれる。
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
		resp := make([]adminShopResponse, 0, len(list)) // nil ではない：[] として marshal される
		for _, detail := range list {
			resp = append(resp, newAdminShopResponse(detail))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleAdminUpdateShop は PUT /admin/shops/{id} を処理する：shop の名前を
// 変更する（編集できる属性は name だけ、Rails parity）。
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

// handleApproveShop は POST /admin/shops/{id}/approve を処理する。リクエスト
// の body は意図的に無視する。この遷移は入力を取らない。
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

// handleRejectShop は POST /admin/shops/{id}/reject を処理する。body（および
// その moderation_note）は任意であり、body が空の場合は null の note で
// reject する。
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
