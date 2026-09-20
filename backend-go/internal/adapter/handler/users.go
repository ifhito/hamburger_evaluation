package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// userNotFoundMessage は、存在しない user、discard 済みの user、および
// 数値でない user の id に共通の 404 body であり、soft delete 済みの
// account が一度も存在しなかったものと区別できないようにする。
const userNotFoundMessage = "User not found"

// userResponse は、GET /users（配列の要素）と PUT /users/{id}（単一の
// オブジェクト）の、token を含まない user の JSON 形式である：issue #16 の
// 仕様（「user 形 + admin フラグ」、issue が引用する frontend の契約の
// レスポンス形状）に従った {id, username, email, admin} であり、
// authUserResponse から token を除いたものである。
type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Email: user.Email, Admin: user.Admin}
}

// updateUserRequest は PUT /users/{id} の {"user":{...}} ラッパーである。
// すべてのフィールドを pointer のままにして、フィールドが存在しない場合
// （nil、変更しない部分更新）と空の場合を区別できるようにしており、
// Rails の strong params に合わせている。{"user":{}} と user キーが存在
// しない場合は、どちらも no-op である。
type updateUserRequest struct {
	User struct {
		Username             *string `json:"username"`
		Email                *string `json:"email"`
		Password             *string `json:"password"`
		PasswordConfirmation *string `json:"password_confirmation"`
	} `json:"user"`
}

// userIDPathValue は {id} の path value をパースする。false は統一された
// user の 404 が既に書き込まれたことを意味する（数値でない id は存在しない
// user とまったく同じに見える。shop/review の id と同じ規約）。
func userIDPathValue(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, userNotFoundMessage)
		return 0, false
	}
	return id, true
}

// writeUserError は users の usecase のエラーを HTTP に対応させる：domain の
// 自己管理の判断は 403、not-found の sentinel は 404（usecase は先に対象を
// load してから所有者を検査するので、存在しない id は所有者でない者に対しても
// 404 になる。これは issue #16 AC2 の find-then-authorize の順序であり、
// 退役した Rails の controller が path の id を無視して current_user に対して
// 動作していたのとは意図的に異なる）、validation は 422、それ以外は 500 である。
func writeUserError(w http.ResponseWriter, op string, err error) {
	var vErr *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, domain.ErrUserNotFound):
		writeError(w, http.StatusNotFound, userNotFoundMessage)
	case errors.As(err, &vErr):
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: vErr.Messages})
	default:
		log.Printf("users: %s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// handleListUsers は GET /users を処理する：kept な user の、公開（認証なし）
// のトップレベルの JSON 配列で、id の昇順である。
func handleListUsers(users *usecase.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := users.List(r.Context())
		if err != nil {
			log.Printf("users: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]userResponse, 0, len(list)) // nil ではない：[] として marshal される
		for _, user := range list {
			resp = append(resp, newUserResponse(user))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleUpdateUser は RequireAuth の背後で PUT /users/{id} を処理する：更新後の
// user を伴う 200。自己管理（admin の例外なし）は domain の判断であり、
// 403 として表面化する。
func handleUpdateUser(users *usecase.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := userIDPathValue(w, r)
		if !ok {
			return
		}
		var req updateUserRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		updated, err := users.Update(r.Context(), viewer, id, usecase.UpdateUserInput{
			Username:             req.User.Username,
			Email:                req.User.Email,
			Password:             req.User.Password,
			PasswordConfirmation: req.User.PasswordConfirmation,
		})
		if err != nil {
			writeUserError(w, "update", err)
			return
		}
		writeJSON(w, http.StatusOK, newUserResponse(updated))
	}
}

// handleDeleteUser は RequireAuth の背後で DELETE /users/{id} を処理する：
// 本人のみの soft delete で、body なしの 204 を返す。
func handleDeleteUser(users *usecase.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := userIDPathValue(w, r)
		if !ok {
			return
		}
		if err := users.Delete(r.Context(), viewer, id); err != nil {
			writeUserError(w, "delete", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
