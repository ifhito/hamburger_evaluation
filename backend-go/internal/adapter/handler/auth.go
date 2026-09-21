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

// authUserResponse は、login と signup の確認（POST /signup/confirm）の成功時に返す
// snake_case の body であり、frontend の契約（domains/auth/types.ts の AuthUserResponse）に従う。
type authUserResponse struct {
	ID       string `json:"id"`
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

// signupAcceptedMessage は、POST /signup の 202 の本文の message である。登録済みの email でも
// 未登録の email でも、同じ値を返す（応答から登録の有無を判別できないようにするため）。
const signupAcceptedMessage = "Confirmation email sent"

// signupTokenInvalidMessage は、確認トークンが期限切れ・存在しない・改ざん・使用済みの
// いずれかのときの 400 のメッセージである（どれなのかは区別できない）。
const signupTokenInvalidMessage = "Confirmation token is invalid or has expired"

type signupConfirmRequest struct {
	Token string `json:"token"`
}

// handleSignup は POST /signup を処理する：入力が有効なら、登録の有無にかかわらず、同じ
// ステータス・本文の 202 {"message":"Confirmation email sent"}（アカウントは確認メールの
// リンクを開いて初めて作られる）、validation の失敗時は 422 {"errors":[...]}。「登録済み」を
// 示すエラーは返さない。
func handleSignup(signups *usecase.Signups) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		err := signups.Request(r.Context(), usecase.SignupInput{
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
		writeJSON(w, http.StatusAccepted, messageResponse{Message: signupAcceptedMessage})
	}
}

// handleSignupConfirm は POST /signup/confirm を処理する：確認トークンが有効なら、従来の
// signup の成功と同じ 201 の本文（user と新しい token）、期限切れ・存在しない・改ざん・
// 使用済みのトークンは、いずれも同じ 400。
func handleSignupConfirm(signups *usecase.Signups) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupConfirmRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		user, token, err := signups.Confirm(r.Context(), req.Token)
		if err != nil {
			if errors.Is(err, domain.ErrSignupTokenInvalid) {
				writeError(w, http.StatusBadRequest, signupTokenInvalidMessage)
				return
			}
			// ログにはトークンの値を含めない（wrap された usecase のエラーにも含まれない）。
			log.Printf("signup confirm: %v", err)
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

type messageResponse struct {
	Message string `json:"message"`
}

// handleLogout は RequireAuth の背後で POST /logout を処理する。認証は
// stateless な JWT なので、server 側で無効化するものは何もない
// （Rails parity）。
func handleLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, messageResponse{Message: "Logged out successfully"})
}
