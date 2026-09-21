package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// signupRequest は POST /signup の body である。PasswordConfirmation は
// pointer のままにして、フィールドが存在しない場合（nil）と空の場合を区別
// できるようにしており、Rails has_secure_password に合わせている。余分な
// 未知のフィールドは無視される。
type signupRequest struct {
	Username             string  `json:"username"`
	Email                string  `json:"email"`
	Password             string  `json:"password"`
	PasswordConfirmation *string `json:"password_confirmation"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// currentUserResponse は、認証済みのユーザー自身の snake_case の表現である。signup・login・
// GET /me で共通に返す。can_moderate は moderation（shop の承認・却下など）ができるかで、
// backend の domain が判断する（frontend は admin から権限を導かない）。
type currentUserResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Admin       bool   `json:"admin"`
	CanModerate bool   `json:"can_moderate"`
}

func newCurrentUserResponse(user domain.User) currentUserResponse {
	return currentUserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Admin:       user.Admin,
		CanModerate: user.CanModerate(),
	}
}

// authUserResponse は、signup と login の成功時に返す snake_case の body で
// あり、frontend の契約（domains/auth/types.ts の SignupResponse）に従う。
type authUserResponse struct {
	currentUserResponse
	Token string `json:"token"`
}

func newAuthUserResponse(user domain.User, token string) authUserResponse {
	return authUserResponse{currentUserResponse: newCurrentUserResponse(user), Token: token}
}

// handleSignup は POST /signup を処理する：user と新しい token を伴う 201、
// 422 {"errors":[...]}（validation の失敗時。email が既に使われている
// 場合を含む）。
func handleSignup(auth *usecase.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		user, token, err := auth.Signup(r.Context(), usecase.SignupInput{
			Username:             req.Username,
			Email:                req.Email,
			Password:             req.Password,
			PasswordConfirmation: req.PasswordConfirmation,
		})
		if err != nil {
			var vErr *domain.ValidationError
			if errors.As(err, &vErr) {
				writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: vErr.Messages})
				return
			}
			// wrap された usecase のエラーには password は含まれない。
			log.Printf("signup: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeJSON(w, http.StatusCreated, newAuthUserResponse(user, token))
	}
}

// handleLogin は POST /login を処理する：user と新しい token を伴う 200、
// 認証情報の形が規則に合わない場合は 422 {"errors":[...]}（signup と同じ形）、
// 規則を満たしたうえで認証情報が誤っている場合は Rails-parity の 401 body。
func handleLogin(auth *usecase.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		user, token, err := auth.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			var vErr *domain.ValidationError
			if errors.As(err, &vErr) {
				writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: vErr.Messages})
				return
			}
			if errors.Is(err, domain.ErrInvalidCredentials) {
				writeError(w, http.StatusUnauthorized, "Invalid email or password")
				return
			}
			log.Printf("login: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, newAuthUserResponse(user, token))
	}
}

// handleMe は RequireAuth の背後で GET /me を処理する：Bearer トークンから解決した現在の
// ユーザー（200）。トークンが無効・期限切れなら RequireAuth が 401 を返す。frontend は、
// トークンの有効性を自分で判断せず、起動時にこの応答でログイン状態を復元する。
func handleMe(w http.ResponseWriter, r *http.Request) {
	viewer, ok := requireViewer(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, newCurrentUserResponse(viewer))
}

type messageResponse struct {
	Message string `json:"message"`
}

// handleLogout は RequireAuth の背後で POST /logout を処理する。認証は
// stateless な JWT なので、server 側で無効化するものは何もない
// （Rails parity）。
func handleLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, messageResponse{Message: "Logged out successfully"})
}
