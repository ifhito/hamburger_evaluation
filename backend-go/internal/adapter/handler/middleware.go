package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// maxRequestBodyBytes caps request bodies at 1 MiB (resource guardrail).
const maxRequestBodyBytes int64 = 1 << 20

// limitBody is the global body-cap middleware. It rejects requests whose
// declared Content-Length exceeds the limit up front with 413 and the
// error JSON shape (covering handlers that never read the body), and wraps
// the body in http.MaxBytesReader so handlers that do read (including
// chunked requests without Content-Length) are also capped.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxRequestBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// viewerKeyType is the unexported context key type for the authenticated
// viewer; collisions with other packages are impossible.
type viewerKeyType struct{}

var viewerKey viewerKeyType

// ViewerFrom returns the authenticated viewer stored by RequireAuth or
// OptionalAuth, and false when the request is anonymous.
func ViewerFrom(ctx context.Context) (domain.User, bool) {
	viewer, ok := ctx.Value(viewerKey).(domain.User)
	return viewer, ok
}

// bearerToken extracts the token from "Authorization: Bearer <token>".
// Per RFC 6750 the scheme is matched case-insensitively and extra
// whitespace between scheme and token is tolerated. A missing header,
// another scheme, a bare token, or an empty token yields ok=false.
func bearerToken(r *http.Request) (string, bool) {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}

// RequireAuth guards a route: without a valid Bearer token of an active
// user it answers the Rails-parity 401 {"error":"Unauthorized"}; on
// success it stores the viewer in the request context. Authentication
// decisions live in the usecase; this only maps them onto HTTP.
func RequireAuth(auth *usecase.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			viewer, err := auth.AuthenticateToken(r.Context(), token)
			if err != nil {
				if errors.Is(err, domain.ErrUnauthenticated) {
					writeError(w, http.StatusUnauthorized, "Unauthorized")
					return
				}
				// Anything else is an infrastructure failure (e.g. a DB
				// error the usecase propagates), a server fault rather
				// than an authentication decision: log it (never the
				// token) and answer 500, consistent with handleSignup
				// and handleLogin (Rails rescues only decode/not-found,
				// so infra failures are 500 there too).
				log.Printf("auth: authenticate token: %v", err)
				writeError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), viewerKey, viewer)))
		})
	}
}

// OptionalAuth stores the viewer in the request context when a valid
// Bearer token of an active user is present, and otherwise lets the
// request continue anonymously — it never rejects.
func OptionalAuth(auth *usecase.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := bearerToken(r); ok {
				viewer, err := auth.AuthenticateToken(r.Context(), token)
				switch {
				case err == nil:
					r = r.WithContext(context.WithValue(r.Context(), viewerKey, viewer))
				case !errors.Is(err, domain.ErrUnauthenticated):
					// Infrastructure failures are swallowed by contract
					// (this middleware never rejects) but must not be
					// silent; the token itself is never logged.
					log.Printf("auth: optional authenticate token: %v", err)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
