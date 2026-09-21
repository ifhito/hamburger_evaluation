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
// 生成 / 成功(200・201・202)以外の状態コードでの writeJSON の直接の呼び出し / http.Error / MCP のツールの失敗(failure)への
// 文字列の直接の受け渡し。
func TestErrorBodiesAreBuiltByTheHelpers(t *testing.T) {
	// 例外(ファイル → 理由)。ここにあるファイルは、対象の書き方を、丸ごと許す。
	exempt := map[string]string{"respond.go": "本文を作る関数そのもの"}
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
					// MCP のツールの失敗も同じ。failure に渡してよいのは、カタログの文言を文字列にした text(...) と、
					// 固定の英語 "internal server error"(5xx 相当)と、日本語で固定した toolResultTooLargeMessage だけ。
					// 文字列の連結・Sprintf・変数は、どれも通さない。
					if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "failure" && len(n.Args) == 1 && !allowedFailureArg(n.Args[0]) {
						t.Errorf("%s: failure に、カタログを通さない文言を渡している(t.failMessage とカタログの文言を使う)", fset.Position(n.Pos()))
					}
				}
				return true
			})
		}
	}
}

// allowedFailureArg は、MCP のツールの失敗(failure)に、直接渡してよい引数かを返す(カタログを通った文言だけ)。
func allowedFailureArg(arg ast.Expr) bool {
	switch a := arg.(type) {
	case *ast.CallExpr:
		if id, ok := a.Fun.(*ast.Ident); ok && id.Name == "text" {
			return true
		}
		// 検証の失敗の文言(domain のカタログを、要求の言語で文字列にしたもの)を、つないだもの: strings.Join(x.Texts(...), …)
		if sel, ok := a.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Join" && len(a.Args) == 2 {
			if inner, ok := a.Args[0].(*ast.CallExpr); ok {
				if isel, ok := inner.Fun.(*ast.SelectorExpr); ok && isel.Sel.Name == "Texts" {
					return true
				}
			}
		}
		return false
	case *ast.BasicLit:
		return a.Kind == token.STRING && a.Value == `"internal server error"`
	case *ast.Ident:
		return a.Name == "toolResultTooLargeMessage"
	}
	return false
}

// TestMessageVariablesAreUsed は、`msgX = apiMsg(keyX)` と宣言した文言の変数が、コードのどこかで使われている
// ことを確かめる(Go は、使われないパッケージの変数を咎めないので、使わなくなった文言が、カタログに残り続ける)。
func TestMessageVariablesAreUsed(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]token.Pos{}
	uses := map[string]int{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				if spec, ok := n.(*ast.ValueSpec); ok {
					for i, name := range spec.Names {
						if i < len(spec.Values) {
							if call, ok := spec.Values[i].(*ast.CallExpr); ok {
								if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "apiMsg" {
									declared[name.Name] = name.Pos()
									continue
								}
							}
						}
					}
					return true
				}
				if id, ok := n.(*ast.Ident); ok {
					uses[id.Name]++
				}
				return true
			})
		}
	}
	if len(declared) == 0 {
		t.Fatal("apiMsg の変数が 1 つも見つからない(検査が空振りしている)")
	}
	for name, pos := range declared {
		// 宣言の名前も 1 回に数えているので、使われているなら、2 回以上ある。
		if uses[name] < 2 {
			t.Errorf("%s: %s が、どこでも使われていない(使わない文言は、変数もカタログも消す)", fset.Position(pos), name)
		}
	}
}
