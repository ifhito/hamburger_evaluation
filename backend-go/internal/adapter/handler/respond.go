// Package handler holds the HTTP boundary: routing, middleware, and
// net/http handlers. It maps errors to the API error shape
// {"error":"..."} (single) or {"errors":[...]} (list) and never contains
// business rules or SQL.
package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

// errorsResponse is the list shape {"errors":[...]}, reserved for
// validation failures (422).
type errorsResponse struct {
	Errors []string `json:"errors"`
}

// writeJSON encodes v as JSON with the given status. Marshal failures for
// the small static payloads used here are programming errors; log and fall
// back to a JSON 500.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Printf("respond: marshal %T: %v", v, err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeError writes the single-error JSON shape {"error":"..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// decodeJSON decodes the request body into dst and reports whether it
// succeeded; on failure the error response has already been written: 413
// when the body-cap MaxBytesReader tripped, 400 for malformed or empty
// JSON. It deliberately tolerates unknown fields — clients send extras
// such as password_confirmation-adjacent fields.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}
