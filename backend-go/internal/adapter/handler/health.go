package handler

import (
	"context"
	"net/http"
	"time"
)

// dbPingTimeout bounds the health-check DB ping (resource guardrail: DB
// calls take the request ctx with an explicit short timeout).
const dbPingTimeout = 2 * time.Second

// Pinger reports database connectivity. *pgxpool.Pool satisfies it; tests
// inject fakes. The health check stays this minimal on purpose — it does
// not need domain/usecase machinery.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PingerFunc adapts a function to Pinger.
type PingerFunc func(ctx context.Context) error

// Ping implements Pinger.
func (f PingerFunc) Ping(ctx context.Context) error { return f(ctx) }

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth serves GET /up: 200 {"status":"ok"} when the DB ping
// succeeds, 503 {"error":"..."} when it fails.
func handleHealth(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), dbPingTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}
