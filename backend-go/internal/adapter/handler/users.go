package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// userNotFoundMessage は、存在しない user、discard 済みの user、および
// UUID の正規形でない user の id に共通の 404 body（GET/PUT/DELETE /users/{id}）であり、
// soft delete 済みの account が一度も存在しなかったものと区別できないように
// する。
const userNotFoundMessage = "User not found"

// userResponse は、PUT /users/{id} の、token を含まない user の JSON 形式である：
// frontend の契約（「user の形 + admin フラグ」のレスポンスの形）に従った {id, username, email, admin} に、自己紹介文（bio）を加えた {id, username, bio, email, admin} であり、
// authUserResponse から token を除いたものである。PUT は本人だけが成功する
// ので、email と admin を常に含めてよい。他人にも返りうる GET の user は
// userProfileResponse を使う。
type userResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Bio: user.Bio, Email: user.Email, Admin: user.Admin}
}

// userProfileResponse は、GET /users/{id} の user の JSON 形式である。公開ビューは
// {id, username, bio} で、本人が閲覧したときだけ {id, username, bio, email, admin} になる。
// Email と Admin は pointer + omitempty で、他人・匿名では nil としてキーごと
// 省かれる（null にはならない）。admin=false は非 nil の pointer なので、本人
// ビューでは "admin":false として出力される。CanEdit は、viewer がこのプロフィールを
// 編集・削除できるか（domain の本人管理ルール）で、本人だけが true。常に出力される。
type userProfileResponse struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Bio      string  `json:"bio"`
	Email    *string `json:"email,omitempty"`
	Admin    *bool   `json:"admin,omitempty"`
	CanEdit  bool    `json:"can_edit"`
}

// newUserProfileResponse は domain.UserProfile を JSON 形式に写すだけである。
// 何を見せるかの判断は domain（User.ProfileFor）が済ませている。
func newUserProfileResponse(profile domain.UserProfile) userProfileResponse {
	return userProfileResponse{ID: profile.ID, Username: profile.Username, Bio: profile.Bio, Email: profile.Email, Admin: profile.Admin, CanEdit: profile.CanEdit}
}

// updateUserRequest は PUT /users/{id} の {"user":{...}} ラッパーである。
// すべてのフィールドを pointer のままにして、フィールドが存在しない場合
// （nil、変更しない部分更新）と空の場合を区別できるようにしており、
// Rails の strong params に合わせている。{"user":{}} と user キーが存在
// しない場合は、どちらも no-op である。
type updateUserRequest struct {
	User struct {
		Username             *string `json:"username"`
		Bio                  *string `json:"bio"`
		Email                *string `json:"email"`
		Password             *string `json:"password"`
		PasswordConfirmation *string `json:"password_confirmation"`
	} `json:"user"`
}

// userIDPathValue は {id} の path value を取り出す。false は統一された
// user の 404 が既に書き込まれたことを意味する（UUID の正規形でない id は
// 存在しない user とまったく同じに見える。shop/review の id と同じ規約）。
// 形式の判定は domain.IsUUID が持つ。
func userIDPathValue(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, http.StatusNotFound, userNotFoundMessage)
		return "", false
	}
	return id, true
}

// writeUserError は users の usecase のエラーを HTTP に対応させる：domain の
// 自己管理の判断は 403、not-found の sentinel は 404（usecase は先に対象を
// load してから所有者を検査するので、存在しない id は所有者でない者に対しても
// 404 になる。これは「先に対象を読み込み、そのあとで権限を確かめる」順序であり、
// 退役した Rails の controller が path の id を無視して current_user に対して
// 動作していたのとは意図的に異なる）、validation は 422、それ以外は 500 である。
func writeUserError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var vErr *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, domain.ErrUserNotFound):
		writeError(w, http.StatusNotFound, userNotFoundMessage)
	case errors.As(err, &vErr):
		writeValidation(w, r, vErr)
	default:
		log.Printf("users: %s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// handleGetUser は GET /users/{id} を処理する：（存在する場合の）viewer から
// 見える user 1 人のビュー、または存在しない・discard 済み・UUID の正規形でない id に
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
			writeUserError(w, r, "get", err)
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
			Bio:                  req.User.Bio,
			Email:                req.User.Email,
			Password:             req.User.Password,
			PasswordConfirmation: req.User.PasswordConfirmation,
		})
		if err != nil {
			writeUserError(w, r, "update", err)
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
			writeUserError(w, r, "delete", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
