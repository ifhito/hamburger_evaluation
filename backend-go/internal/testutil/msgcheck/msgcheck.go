// Package msgcheck は、利用者に見える文言のカタログ(キー → 英語・日本語の書式)の、構造の検査である。
// domain と handler のカタログが、同じ約束(両言語がそろう・値の並びが同じ・キーと使われ方が一致する)を
// 守ることを、同じ実装で確かめる。テストからだけ使う。
package msgcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// Entry は、1 つの文言の、言語ごとの書式である。
type Entry struct {
	EN, JA string
}

// Config は、検査の対象である。
type Config struct {
	// Catalog は、キー → 書式のカタログである。
	Catalog map[string]Entry
	// KeyFile は、`key…` の定数(キーの名前 → キーの値)を書いたファイルである。
	KeyFile string
	// Dirs は、Msg(key, args...) の呼び出しを探すディレクトリである(テスト以外の .go)。
	Dirs []string
	// Constructor は、文言を作る関数の名前である(空なら Msg)。Type は、その文言の型の名前である(空なら Message)。
	Constructor, Type string
	// DirectOK は、Type{...}(中身を書いたもの)を、Constructor を通さずに直接書いてよいファイル名である。
	DirectOK []string
}

// Run は、次を確かめる: 英語と日本語の両方がある(日本語は、日本語の文字を含む。英語のコピーのままでない) /
// 書式の値の並びが両言語で同じ / キーの定数とカタログが一致する / コードの Msg のキーがカタログにある /
// 引数の数と(リテラルの)型が書式と合う / カタログの文言がコードのどこかで使われている /
// Msg を通さない Message{...} がない。
func Run(t *testing.T, c Config) {
	t.Helper()
	if c.Constructor == "" {
		c.Constructor = "Msg"
	}
	if c.Type == "" {
		c.Type = "Message"
	}
	t.Run("両言語がそろっている", func(t *testing.T) {
		for key, e := range c.Catalog {
			if strings.TrimSpace(e.EN) == "" {
				t.Errorf("キー %q の英語の文言が空である", key)
			}
			if strings.TrimSpace(e.JA) == "" {
				t.Errorf("キー %q の日本語の文言が空である(キーを足したら、英語と日本語の両方を書く)", key)
			} else if !hasJapanese(e.JA) {
				t.Errorf("キー %q の日本語の文言 %q に、日本語の文字がない(英語のコピーのままでは、日本語の利用者に英語が出る)", key, e.JA)
			}
		}
	})
	t.Run("両言語で同じ値を入れる", func(t *testing.T) {
		for key, e := range c.Catalog {
			if en, ja := formatArgs(t, e.EN), formatArgs(t, e.JA); !reflect.DeepEqual(en, ja) {
				t.Errorf("キー %q: 英語の値の使い方 %v と、日本語 %v が食い違う(日本語でも、同じ値を入れる)", key, en, ja)
			}
		}
	})
	consts := keyConstants(t, c.KeyFile)
	t.Run("キーの定数とカタログが一致する", func(t *testing.T) {
		values := map[string]bool{}
		for name, value := range consts {
			values[value] = true
			if _, ok := c.Catalog[value]; !ok {
				t.Errorf("定数 %s のキー %q が、カタログにない", name, value)
			}
		}
		for key := range c.Catalog {
			if !values[key] {
				t.Errorf("カタログのキー %q に対応する `key…` の定数がない", key)
			}
		}
	})
	t.Run("コードの使い方がカタログと合う", func(t *testing.T) {
		calls, direct := scan(t, c, consts)
		if len(calls) == 0 {
			t.Fatal("Msg(...) の呼び出しが 1 つも見つからない(検査が空振りしている)")
		}
		used := map[string]bool{}
		for _, call := range calls {
			e, ok := c.Catalog[call.key]
			if !ok {
				t.Errorf("%s: Msg のキーが、カタログにない(またはキーを、リテラル・key… の定数で書いていない)", call.pos)
				continue
			}
			used[call.key] = true
			uses := formatArgs(t, e.EN)
			if want := argCount(uses); len(call.args) != want {
				t.Errorf("%s: キー %q の書式は引数を %d 個使うが、%d 個渡している", call.pos, call.key, want, len(call.args))
			}
			for i, arg := range call.args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok {
					continue
				}
				for _, use := range uses {
					n, verb, _ := strings.Cut(use, ":")
					if n == strconv.Itoa(i+1) && (verb == "d") != (lit.Kind == token.INT) {
						t.Errorf("%s: キー %q の %d 番目の引数(%s)の型が、書式の %%%s と合わない", call.pos, call.key, i+1, lit.Value, verb)
					}
				}
			}
		}
		for key := range c.Catalog {
			if !used[key] {
				t.Errorf("カタログのキー %q が、コードのどこでも使われていない(使われていない文言は、消す)", key)
			}
		}
		for _, pos := range direct {
			t.Errorf("%s: Message は Msg(key, args...) で作る(直接書くと、キーの検査をすり抜ける)", pos)
		}
	})
}

// hasJapanese は、s に、ひらがな・カタカナ・漢字のいずれかがあるかを返す。
func hasJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

// formatArgs は、fmt の書式から、引数の使われ方(「何番目の引数を、どの型で」)の一覧を取り出す。
// 日本語で値の順序が変わるとき(%[2]d)も、同じ値が入ることを比べるために使う。
func formatArgs(t *testing.T, format string) []string {
	t.Helper()
	verb := regexp.MustCompile(`%(?:\[(\d+)\])?[+\-# 0]*\d*(?:\.\d+)?([a-zA-Z])`)
	var uses []string
	next := 1
	for _, m := range verb.FindAllStringSubmatch(format, -1) {
		idx := next
		if m[1] != "" {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("書式 %q の引数の番号が読めない: %v", format, err)
			}
			idx = n
		}
		uses = append(uses, strconv.Itoa(idx)+":"+m[2])
		next = idx + 1
	}
	sort.Strings(uses)
	return uses
}

// argCount は、書式が使う引数の数(いちばん後ろの引数の番号)である。
func argCount(uses []string) int {
	most := 0
	for _, use := range uses {
		n, _ := strconv.Atoi(strings.SplitN(use, ":", 2)[0])
		if n > most {
			most = n
		}
	}
	return most
}

// keyConstants は、file の `key…` の定数(名前 → キーの値)を、構文木から読む。
func keyConstants(t *testing.T, file string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("%s を読めない: %v", file, err)
	}
	consts := map[string]string{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "key") {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("定数 %s が文字列のリテラルでない", name.Name)
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("定数 %s の値が読めない: %v", name.Name, err)
				}
				consts[name.Name] = value
			}
		}
	}
	return consts
}

// call は、コードの中の Msg(...) の呼び出し 1 つである。
type call struct {
	pos  string
	key  string
	args []ast.Expr
}

// scan は、c.Dirs の(テスト以外の)コードから、Msg(...) の呼び出しと、Msg を通さずに直接書かれた
// Type{...}(中身を書いたもの)を集める。キーは、文字列のリテラルか、key… の定数だけを読む。
func scan(t *testing.T, c Config, consts map[string]string) (calls []call, direct []string) {
	t.Helper()
	for _, dir := range c.Dirs {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			t.Fatalf("%s を読めない: %v", dir, err)
		}
		for _, pkg := range pkgs {
			for path, file := range pkg.Files {
				allowed := false
				for _, name := range c.DirectOK {
					allowed = allowed || strings.HasSuffix(path, "/"+name) || path == name
				}
				ast.Inspect(file, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.CallExpr:
						if len(n.Args) == 0 || !isName(n.Fun, c.Constructor) {
							return true
						}
						var key string
						switch arg := n.Args[0].(type) {
						case *ast.BasicLit:
							key, _ = strconv.Unquote(arg.Value)
						case *ast.Ident:
							key = consts[arg.Name]
						}
						calls = append(calls, call{pos: fset.Position(n.Pos()).String(), key: key, args: n.Args[1:]})
					case *ast.CompositeLit:
						if len(n.Elts) > 0 && isName(n.Type, c.Type) && !allowed {
							direct = append(direct, fset.Position(n.Pos()).String())
						}
					}
					return true
				})
			}
		}
	}
	return calls, direct
}

// isName は、expr が name(または pkg.name)かを返す。
func isName(expr ast.Expr, name string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return e.Sel.Name == name
	}
	return false
}
