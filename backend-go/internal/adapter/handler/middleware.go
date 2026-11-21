package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// headerTrackingWriter は、下流がステータスコードや本文をすでに書き始めたかを記録する。
// http.MaxBytesReader などが元の ResponseWriter を辿れるよう Unwrap を実装する。
type headerTrackingWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *headerTrackingWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *headerTrackingWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

func (w *headerTrackingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// recoverPanic は、handler の中で予期せぬ panic が起きても接続を素で切断せず、スタックトレースをログに残して
// 500 JSON({"error":"internal server error"})を返す最外周の middleware である。
// net/http の規約に従い http.ErrAbortHandler だけは握りつぶさず再送出する。
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &headerTrackingWriter{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			log.Printf("http: panic recovered: %v\n%s", rec, debug.Stack())
			if !tw.wroteHeader {
				writeInternalError(w)
			}
		}()
		next.ServeHTTP(tw, r)
	})
}

// maxRequestBodyBytes は、すべての経路・Content-Type で本文全体を 10 MiB に制限する。
// 写真自体のサイズや各入力項目の検査は、個別の処理で引き続き行う。
const maxRequestBodyBytes int64 = 10 << 20

// limitBody はグローバルな body cap の middleware である。宣言された
// Content-Length が上限を超える request は、事前に 413 とエラー JSON 形式で
// 拒否し（body を読まない handler も対象になる）、さらに body を
// http.MaxBytesReader で包むことで、body を読む handler（Content-Length の
// ない chunked request を含む）も上限の対象にする。
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := maxRequestBodyBytes
		if r.ContentLength > limit {
			writeError(w, r, http.StatusRequestEntityTooLarge, msgBodyTooLarge)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// viewerKeyType は、認証済みの viewer 用の非公開の context キー型である。
// 他の package との衝突は起こりえない。
type viewerKeyType struct{}

var viewerKey viewerKeyType

// ViewerFrom は RequireAuth または OptionalAuth が格納した認証済みの viewer を
// 返し、request が匿名の場合は false を返す。
func ViewerFrom(ctx context.Context) (domain.User, bool) {
	viewer, ok := ctx.Value(viewerKey).(domain.User)
	return viewer, ok
}

// bearerToken は "Authorization: Bearer <token>" から token を取り出す。
// RFC 6750 に従い、scheme は大文字小文字を区別せずに照合され、scheme と
// token の間の余分な空白は許容される。ヘッダーがない、別の scheme である、
// scheme のない token 単体である、または token が空である場合は ok=false に
// なる。
func bearerToken(r *http.Request) (string, bool) {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}

// RequireAuth は route を保護する：active な user の有効な Bearer token が
// ない場合は Rails-parity の 401 {"error":"Unauthorized"} を返し、成功した
// 場合は viewer を request の context に格納する。認証の判断は usecase に
// あり、ここではそれを HTTP に対応させるだけである。
func RequireAuth(auth *usecase.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, r, http.StatusUnauthorized, msgUnauthorized)
				return
			}
			viewer, err := auth.AuthenticateToken(r.Context(), token)
			if err != nil {
				if errors.Is(err, domain.ErrUnauthenticated) {
					writeError(w, r, http.StatusUnauthorized, msgUnauthorized)
					return
				}
				// それ以外は infrastructure の障害（例：usecase が
				// 伝播する DB エラー）であり、認証の判断ではなく
				// server 側の不具合である。ログに記録し（token は
				// 決して記録しない）、handleSignup および
				// handleLogin と同様に 500 を返す（Rails が rescue
				// するのは decode/not-found だけなので、Rails でも
				// infra の障害は 500 になる）。
				log.Printf("auth: authenticate token: %v", err)
				writeInternalError(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), viewerKey, viewer)))
		})
	}
}

// OptionalAuth は、active な user の有効な Bearer token がある場合は viewer を
// request の context に格納し、そうでなければ request を匿名のまま続行させる。
// 決して拒否しない。
func OptionalAuth(auth *usecase.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := bearerToken(r); ok {
				viewer, err := auth.AuthenticateToken(r.Context(), token)
				switch {
				case err == nil:
					r = r.WithContext(context.WithValue(r.Context(), viewerKey, viewer))
				case !errors.Is(err, domain.ErrUnauthenticated):
					// infrastructure の障害は契約上握りつぶされる
					// （この middleware は決して拒否しない）が、何も
					// 記録しないままにしてはならない。token 自体は
					// 決してログに記録しない。
					log.Printf("auth: optional authenticate token: %v", err)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
