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

// authUserResponse は、signup と login の成功時に返す snake_case の body で
// あり、frontend の契約（shared/lib/types/auth.ts の SignupResponse）に従う。
type authUserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
	Token    string `json:"token"`
}

func newAuthUserResponse(user domain.User, token string) authUserResponse {
	return authUserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Admin:    user.Admin,
		Token:    token,
	}
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
// 認証情報が誤っている場合は Rails-parity の 401 body。
func handleLogin(auth *usecase.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		user, token, err := auth.Login(r.Context(), req.Email, req.Password)
		if err != nil {
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

type messageResponse struct {
	Message string `json:"message"`
}

// handleLogout は RequireAuth の背後で POST /logout を処理する。認証は
// stateless な JWT なので、server 側で無効化するものは何もない
// （Rails parity）。
func handleLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, messageResponse{Message: "Logged out successfully"})
}
