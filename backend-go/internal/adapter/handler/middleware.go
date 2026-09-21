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

// maxRequestBodyBytes は request body を 1 MiB に制限する
// （resource guardrail）。
const maxRequestBodyBytes int64 = 1 << 20

// maxReviewRequestBodyBytes は review の投稿 body（multipart のみ）を 6 MiB に
// 制限する：5 MiB の写真に加え、フィールドと multipart のフレーミングの分の
// 余裕がある。写真自体は引き続き独自の 5 MiB の上限で検査され、422 を返す
// のはそちらである。この cap は暴走した body を 413 で止めるだけである。
const maxReviewRequestBodyBytes int64 = 6 << 20

// bodyLimit は request の body の上限を返す：より大きな上限を得るのは、
// review の書き込み endpoint（POST /reviews、PUT /reviews/{id}）に対する
// multipart/form-data の request（写真を含みうる）だけである。JSON を含む
// それ以外の Content-Type は、review の書き込みでも 1 MiB のままである
// （コメントの上限が 2,000 文字なので、JSON に 6 MiB は要らない）。
func bodyLimit(r *http.Request) int64 {
	isReviewWrite := (r.Method == http.MethodPost && r.URL.Path == "/reviews") ||
		(r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/reviews/"))
	if isReviewWrite && isMultipart(r) {
		return maxReviewRequestBodyBytes
	}
	return maxRequestBodyBytes
}

// limitBody はグローバルな body cap の middleware である。宣言された
// Content-Length が上限を超える request は、事前に 413 とエラー JSON 形式で
// 拒否し（body を読まない handler も対象になる）、さらに body を
// http.MaxBytesReader で包むことで、body を読む handler（Content-Length の
// ない chunked request を含む）も上限の対象にする。
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := bodyLimit(r)
		if r.ContentLength > limit {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
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
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			viewer, err := auth.AuthenticateToken(r.Context(), token)
			if err != nil {
				if errors.Is(err, domain.ErrUnauthenticated) {
					writeError(w, http.StatusUnauthorized, "Unauthorized")
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
				writeError(w, http.StatusInternalServerError, "internal server error")
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
