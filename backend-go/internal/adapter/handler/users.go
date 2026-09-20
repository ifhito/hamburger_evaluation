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
// 数値でない user の id に共通の 404 body（GET/PUT/DELETE /users/{id}）であり、
// soft delete 済みの account が一度も存在しなかったものと区別できないように
// する。
const userNotFoundMessage = "User not found"

// userResponse は、PUT /users/{id} の、token を含まない user の JSON 形式である：
// issue #16 の仕様（「user 形 + admin フラグ」、issue が引用する frontend の契約の
// レスポンス形状）に従った {id, username, email, admin} であり、
// authUserResponse から token を除いたものである。PUT は本人だけが成功する
// ので、email と admin を常に含めてよい。他人にも返りうる GET の user は
// userProfileResponse を使う。
type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Email: user.Email, Admin: user.Admin}
}

// userProfileResponse は、GET /users（配列の要素）と GET /users/{id} の user の
// JSON 形式である。公開ビューは {id, username} で、本人が閲覧したときだけ
// {id, username, email, admin} になる。Email と Admin は pointer + omitempty で、
// 他人・匿名では nil としてキーごと省かれる（null にはならない）。admin=false は
// 非 nil の pointer なので、本人ビューでは "admin":false として出力される。
type userProfileResponse struct {
	ID       int64   `json:"id"`
	Username string  `json:"username"`
	Email    *string `json:"email,omitempty"`
	Admin    *bool   `json:"admin,omitempty"`
}

// newUserProfileResponse は domain.UserProfile を JSON 形式に写すだけである。
// 何を見せるかの判断は domain（User.ProfileFor）が済ませている。
func newUserProfileResponse(profile domain.UserProfile) userProfileResponse {
	return userProfileResponse{ID: profile.ID, Username: profile.Username, Email: profile.Email, Admin: profile.Admin}
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

// handleListUsers は GET /users を処理する：kept な user の、（存在する場合の）
// viewer から見えるビューのトップレベルの JSON 配列で、全件を id の昇順で
// 返す（ページネーションなし）。認証は任意（OptionalAuth）で、email と admin が
// 入るのは viewer 本人の要素だけである。
func handleListUsers(users *usecase.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := users.List(r.Context(), viewerPtr(r))
		if err != nil {
			log.Printf("users: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]userProfileResponse, 0, len(list)) // nil ではない：[] として marshal される
		for _, profile := range list {
			resp = append(resp, newUserProfileResponse(profile))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleGetUser は GET /users/{id} を処理する：（存在する場合の）viewer から
// 見える user 1 人のビュー、または存在しない・discard 済み・数値でない id に
// 対する統一された 404。認証は任意（OptionalAuth）で、email と admin が入る
// のは viewer 本人が閲覧したときだけである。
func handleGetUser(users *usecase.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDPathValue(w, r)
		if !ok {
			return
		}
		profile, err := users.Get(r.Context(), viewerPtr(r), id)
		if err != nil {
			writeUserError(w, "get", err)
			return
		}
		writeJSON(w, http.StatusOK, newUserProfileResponse(profile))
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
