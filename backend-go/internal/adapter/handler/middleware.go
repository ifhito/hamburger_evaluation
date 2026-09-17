package handler

import "net/http"

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
