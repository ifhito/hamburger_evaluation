package handler

import (
	"context"
	"log"
	"net/http"
	"time"
)

// dbPingTimeout は health check の DB ping に上限を設ける（resource
// guardrail：DB 呼び出しは request の ctx に明示的な短い timeout を付けて
// 使う）。
const dbPingTimeout = 2 * time.Second

// Pinger は database の接続性を報告する。*pgxpool.Pool がこれを満たし、
// テストでは fake を注入する。health check は意図的にここまで最小にして
// いる。domain/usecase の仕組みは必要ない。
type Pinger interface {
	Ping(ctx context.Context) error
}

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth は GET /up を処理する：DB の ping が成功したときは
// 200 {"status":"ok"}、失敗したときは 503 {"error":"..."}。
func handleHealth(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), dbPingTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			log.Printf("health: db ping failed: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}
