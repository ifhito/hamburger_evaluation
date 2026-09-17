// Package handler holds the HTTP boundary: routing, middleware, and
// net/http handlers. It maps errors to the API error shape
// {"error":"..."} (single) or {"errors":[...]} (list) and never contains
// business rules or SQL.
package handler

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

// errorsResponse is the list-shaped error body, used for validation errors
// in later stories.
type errorsResponse struct {
	Errors []string `json:"errors"`
}

// writeJSON encodes v as JSON with the given status. Marshal failures for
// the small static payloads used here are programming errors; fall back to
// a plain 500.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
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
