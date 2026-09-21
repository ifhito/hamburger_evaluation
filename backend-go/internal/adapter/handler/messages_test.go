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
	msgcheck.Run(t, msgcheck.Config{Catalog: entries, KeyFile: "messages.go", Dirs: []string{"."}})
}

// TestErrorBodiesAreBuiltByTheHelpers は、エラーの本文({"error":…}・{"errors":[…]})を、respond.go の
// 関数(writeError・writeErrorList・writeValidation・writeInternalError)だけが作ることを確かめる。
// 文字列を直接書いて本文を作ると、利用者の言語に合わせる仕組みをすり抜けて、英語のままになる。
func TestErrorBodiesAreBuiltByTheHelpers(t *testing.T) {
	// 例外: 5xx の固定の文字列は、言語によらず英語である。
	allowed := map[string]string{
		"respond.go": "本文を作る関数そのもの",
		"health.go":  "503 database unavailable(5xx は英語のまま)",
		// S51 の 3/3 で、カタログに移す(OAuth の許可の画面・Google・MCP)。移したら、ここから消す。
		"google_login.go":  "Google の案内の文言",
		"mcp.go":           "Insufficient scope",
		"oauth_consent.go": "許可の画面の文言",
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if _, ok := allowed[path]; ok {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if id, ok := lit.Type.(*ast.Ident); ok && (id.Name == "errorResponse" || id.Name == "errorsResponse") {
					t.Errorf("%s: %s を直接作っている(writeError・writeErrorList・writeValidation を使う)", fset.Position(lit.Pos()), id.Name)
				}
				return true
			})
		}
	}
}
