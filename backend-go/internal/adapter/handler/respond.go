// Package handler holds the HTTP boundary: routing, middleware, and
// net/http handlers. It maps errors to the API error shape
// {"error":"..."} (single) or {"errors":[...]} (list) and never contains
// business rules or SQL.
package handler

import (
	"encoding/json"
	"log"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
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
