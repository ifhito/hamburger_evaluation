package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// signupRequest is the POST /signup body. PasswordConfirmation stays a
// pointer so an absent field (nil) is distinguished from an empty one,
// matching Rails has_secure_password. Extra unknown fields are ignored.
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

// authUserResponse is the snake_case body of successful signup and login,
// per the frontend contract (SignupResponse in domains/auth/types.ts).
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

// handleSignup serves POST /signup: 201 with the user and a fresh token,
// 422 {"errors":[...]} on validation failure (including a taken email).
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
			// Wrapped usecase errors never carry the password.
			log.Printf("signup: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeJSON(w, http.StatusCreated, newAuthUserResponse(user, token))
	}
}

// handleLogin serves POST /login: 200 with the user and a fresh token, or
// the Rails-parity 401 body on bad credentials.
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

// handleLogout serves POST /logout behind RequireAuth. Auth is stateless
// JWT, so there is nothing to invalidate server-side (Rails parity).
func handleLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, messageResponse{Message: "Logged out successfully"})
}
