package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/msgcheck"
)

// TestCatalogStructure は、handler のカタログの構造(両言語・値の並び・キーの定数・コードの使い方)を確かめる。
// 検査の中身は internal/testutil/msgcheck(domain のカタログと同じ実装)にある。
func TestCatalogStructure(t *testing.T) {
	entries := map[string]msgcheck.Entry{}
	for key, e := range catalog {
		entries[key] = msgcheck.Entry{EN: e.EN, JA: e.JA}
	}
	msgcheck.Run(t, msgcheck.Config{
		Catalog:     entries,
		KeyFile:     "messages.go",
		Dirs:        []string{"."},
		Constructor: "apiMsg",
		Type:        "apiMessage",
		DirectOK:    []string{"messages.go"},
	})
}

// TestErrorBodiesAreBuiltByTheHelpers は、利用者に見えるエラーの本文を、respond.go の関数(writeError・
// writeErrorList・writeValidation)だけが作ることを確かめる。文字列を直接書いて本文を作ると、利用者の言語に
// 合わせる仕組みをすり抜けて、英語のままになる。次の書き方を見つける: errorResponse・errorsResponse の直接の
// 生成 / 成功(200・201・202)以外の状態コードでの writeJSON の直接の呼び出し / http.Error。
func TestErrorBodiesAreBuiltByTheHelpers(t *testing.T) {
	// 例外(ファイル → 理由)。ここにあるファイルは、対象の書き方を、丸ごと許す。
	exempt := map[string]string{
		"respond.go": "本文を作る関数そのもの",
		// 文言をカタログに移す作業の途中のファイル。移したら、ここから消す。
		"google_login.go":  "Google の案内の文言",
		"mcp.go":           "Insufficient scope",
		"oauth_consent.go": "許可の画面の文言",
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	isHTTP := func(expr ast.Expr, name string) bool {
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == "http"
	}
	succeeds := func(status ast.Expr) bool {
		for _, name := range []string{"StatusOK", "StatusCreated", "StatusAccepted"} {
			if isHTTP(status, name) {
				return true
			}
		}
		return false
	}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if _, ok := exempt[path]; ok {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CompositeLit:
					// 5xx(health.go の 503)は、言語によらず英語の固定の文字列なので、ここだけ、直接作ってよい。
					if id, ok := n.Type.(*ast.Ident); ok && (id.Name == "errorResponse" || id.Name == "errorsResponse") && path != "health.go" {
						t.Errorf("%s: %s を直接作っている(writeError・writeErrorList・writeValidation を使う)", fset.Position(n.Pos()), id.Name)
					}
				case *ast.CallExpr:
					if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "writeJSON" && len(n.Args) >= 2 && !succeeds(n.Args[1]) {
						if path == "health.go" && isHTTP(n.Args[1], "StatusServiceUnavailable") {
							return true
						}
						t.Errorf("%s: 成功以外の状態コードで writeJSON を直接呼んでいる(writeError・writeErrorList・writeValidation を使う)", fset.Position(n.Pos()))
					}
					if isHTTP(n.Fun, "Error") {
						t.Errorf("%s: http.Error は使わない(本文が、言語の切り替えをすり抜ける)", fset.Position(n.Pos()))
					}
				}
				return true
			})
		}
	}
}
