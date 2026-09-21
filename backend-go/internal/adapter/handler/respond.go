// Package handler は HTTP の境界（routing、middleware、net/http の
// handler）を担う。エラーを API のエラー形式 {"error":"..."}（単一）または
// {"errors":[...]}（リスト）に対応させ、業務ルールや SQL は決して含まない。
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
)

type errorResponse struct {
	Error string `json:"error"`
}

// errorsResponse は {"errors":[...]} というリスト形式であり、validation の
// 失敗（422）専用に予約されている。
type errorsResponse struct {
	Errors []string `json:"errors"`
}

// writeJSON は v を、指定された status の JSON としてエンコードする。ここで
// 使う小さな静的 payload の marshal の失敗はプログラミングエラーであるため、
// ログに記録し、JSON の 500 にフォールバックする。
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

// hasMoreHeader は、一覧（GET /shops・GET /reviews）が次のページを持つかを返す
// レスポンスヘッダーの名前である。配列のレスポンスの形（既存の契約）を変えないために、
// 本文ではなくヘッダーで返す。値は "true" か "false"。
const hasMoreHeader = "X-Has-More"

// setHasMore は、本文を書く前に、次のページの有無をヘッダーに設定する。
func setHasMore(w http.ResponseWriter, hasMore bool) {
	w.Header().Set(hasMoreHeader, strconv.FormatBool(hasMore))
}

// writeError は単一エラーの JSON 形式 {"error":"..."} を書き込む。
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// decodeJSON は request の body を dst にデコードし、成功したかどうかを返す。
// 失敗した場合、エラーレスポンスは既に書き込まれている：body cap の
// MaxBytesReader が作動したときは 413、不正または空の JSON には 400 である。
// 未知のフィールドは意図的に許容する。client は password_confirmation に
// 隣接するフィールドのような余分なフィールドを送ってくるためである。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeJSONBody(w, r, dst, false)
}

// decodeOptionalJSON は、body が任意の endpoint（例：reject の moderation
// note）向けの decodeJSON の派生版である。空の body は 400 を返す代わりに
// 成功となり、dst には手を加えない。これは、存在しない params が単に nil に
// なる Rails に合わせたものである。それ以外は decodeJSON と同じ動作をする。
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeJSONBody(w, r, dst, true)
}

// decodeJSONBody は decodeJSON と decodeOptionalJSON の共通の中核である。
// allowEmpty により、空の body（最初の Decode で io.EOF になる）は、dst に
// 手を加えない成功として扱われる。
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return true // 空の body：デコードするものがない
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	// JSON 値の後ろに続くゴミを拒否する：2 回目の Decode は正常な
	// end-of-stream に達しなければならず、そうでなければ body は単一の
	// JSON ドキュメントではなかったことになる。
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}
