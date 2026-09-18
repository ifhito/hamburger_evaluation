package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// userNotFoundMessage is the shared 404 body for missing, discarded, and
// non-numeric user ids, so soft-deleted accounts are indistinguishable
// from never-existing ones.
const userNotFoundMessage = "User not found"

// userResponse is the token-less user JSON shape of GET /users (array
// elements) and PUT /users/{id} (single object): {id, username, email,
// admin} per issue #16's 仕様 (「user 形 + admin フラグ」, the response
// shape of the issue's cited frontend contract) — authUserResponse minus
// token.
type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Email: user.Email, Admin: user.Admin}
}

// updateUserRequest is the {"user":{...}} wrapper of PUT /users/{id}.
// Every field stays a pointer so an absent field (nil, left untouched —
// partial update) is distinguished from an empty one, matching Rails
// strong params; {"user":{}} and an absent user key are both no-ops.
type updateUserRequest struct {
	User struct {
		Username             *string `json:"username"`
		Email                *string `json:"email"`
		Password             *string `json:"password"`
		PasswordConfirmation *string `json:"password_confirmation"`
	} `json:"user"`
}

// userIDPathValue parses the {id} path value; false means the uniform
// user 404 was already written (non-numeric ids look exactly like missing
// users, the shop/review-id convention).
func userIDPathValue(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, userNotFoundMessage)
		return 0, false
	}
	return id, true
}

// writeUserError maps the users usecase errors onto HTTP: the domain
// self-management decision to 403, the not-found sentinel to 404 (checked
// after the load, so a nonexistent id 404s even for a non-owner — the
// find-then-authorize order of issue #16 AC2; this deliberately diverges
// from this branch's Rails controller, which ignores the path id),
// validation to 422, anything else to 500.
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

// handleListUsers serves GET /users: the public (no auth) top-level JSON
// array of kept users, id ascending.
func handleListUsers(users *usecase.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := users.List(r.Context())
		if err != nil {
			log.Printf("users: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]userResponse, 0, len(list)) // non-nil: marshals as []
		for _, user := range list {
			resp = append(resp, newUserResponse(user))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleUpdateUser serves PUT /users/{id} behind RequireAuth: 200 with
// the updated user. Self-management (no admin pass) is the domain's
// decision surfaced as 403.
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

// handleDeleteUser serves DELETE /users/{id} behind RequireAuth: a
// self-only soft delete answered with 204 and no body.
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
