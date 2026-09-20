package handler

import (
	"net/http"
	"strings"
)

// PhotoFileServer は disk 上の photo dir を読み取り専用で配信する：
// http.Dir に対する http.FileServer は request を既に root に閉じ込めて
// おり（".." は決して外へ出られない）、この wrapper はディレクトリへの
// request を一覧表示ではなく 404 にするので、保存された key を列挙できない。
// NewRouter の StripPrefix("/photos/") の背後にマウントされることを前提と
// しており、これが見る path は root からの相対である。cmd/api ではなく
// ここに置いているのは、router のテストが本番で配線される wrapper と
// まったく同じものを検証できるようにするためである。
func PhotoFileServer(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; p == "" || strings.HasSuffix(p, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
