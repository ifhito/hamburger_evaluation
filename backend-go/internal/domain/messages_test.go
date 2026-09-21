package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

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

func TestCatalogEntriesHaveBothLanguages(t *testing.T) {
	for key, entry := range catalog {
		if strings.TrimSpace(entry.EN) == "" {
			t.Errorf("キー %q の英語の文言が空である", key)
		}
		if strings.TrimSpace(entry.JA) == "" {
			t.Errorf("キー %q の日本語の文言が空である(キーを足したら、英語と日本語の両方を書く)", key)
		}
	}
}

func TestCatalogEntriesUseTheSameValuesInBothLanguages(t *testing.T) {
	for key, entry := range catalog {
		if en, ja := formatArgs(t, entry.EN), formatArgs(t, entry.JA); !reflect.DeepEqual(en, ja) {
			t.Errorf("キー %q: 英語の値の使い方 %v と、日本語 %v が食い違う(日本語でも、同じ値を入れる)", key, en, ja)
		}
	}
}

// keyConstants は、messages.go の `key…` の定数(名前 → キーの値)を、構文木から読む。
func keyConstants(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "messages.go", nil, 0)
	if err != nil {
		t.Fatalf("messages.go を読めない: %v", err)
	}
	consts := map[string]string{}
	for _, decl := range file.Decls {
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

func TestCatalogAndKeyConstantsMatch(t *testing.T) {
	values := map[string]bool{}
	for name, value := range keyConstants(t) {
		values[value] = true
		if _, ok := catalog[value]; !ok {
			t.Errorf("定数 %s のキー %q が、カタログにない", name, value)
		}
	}
	for key := range catalog {
		if !values[key] {
			t.Errorf("カタログのキー %q に対応する `key…` の定数がない(使われていない文言は、消す)", key)
		}
	}
}

// msgCall は、コードの中の Msg(...) の呼び出し 1 つである。
type msgCall struct {
	pos  string
	key  string
	args []ast.Expr
}

// scanMessages は、domain と usecase の(テスト以外の)コードから、Msg(...) の呼び出しと、Msg を通さずに
// 直接書かれた Message{...} を集める。キーは、文字列のリテラルか、messages.go の `key…` の定数だけを許す。
func scanMessages(t *testing.T) (calls []msgCall, direct []string) {
	t.Helper()
	consts := keyConstants(t)
	for _, dir := range []string{".", filepath.Join("..", "usecase")} {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			t.Fatalf("%s を読めない: %v", dir, err)
		}
		for _, pkg := range pkgs {
			for path, file := range pkg.Files {
				ast.Inspect(file, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.CallExpr:
						if len(n.Args) == 0 || !isMsgCall(n.Fun) {
							return true
						}
						var key string
						switch arg := n.Args[0].(type) {
						case *ast.BasicLit:
							key, _ = strconv.Unquote(arg.Value)
						case *ast.Ident:
							key = consts[arg.Name]
						}
						calls = append(calls, msgCall{pos: fset.Position(n.Pos()).String(), key: key, args: n.Args[1:]})
					case *ast.CompositeLit:
						if isMessageType(n.Type) && path != "message.go" {
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

// argCount は、書式が使う引数の数(いちばん後ろの引数の番号)である。
func argCount(t *testing.T, format string) int {
	t.Helper()
	most := 0
	for _, use := range formatArgs(t, format) {
		n, _ := strconv.Atoi(strings.SplitN(use, ":", 2)[0])
		if n > most {
			most = n
		}
	}
	return most
}

// TestMessagesInTheCodeAreInTheCatalog は、コードが Msg(...) で作る文言について、次を確かめる。
// キーがカタログにある / 引数の数が、書式の値の数と同じ / 使われていない文言がない /
// Msg を通さない Message{...} がない(通さないと、キーの検査をすり抜ける)。
func TestMessagesInTheCodeAreInTheCatalog(t *testing.T) {
	calls, direct := scanMessages(t)
	if len(calls) == 0 {
		t.Fatal("Msg(...) の呼び出しが 1 つも見つからない(検査が空振りしている)")
	}
	used := map[string]bool{}
	for _, call := range calls {
		entry, ok := catalog[call.key]
		if !ok {
			t.Errorf("%s: Msg のキーが、カタログにない(またはキーを、リテラル・key… の定数で書いていない)", call.pos)
			continue
		}
		used[call.key] = true
		if want := argCount(t, entry.EN); len(call.args) != want {
			t.Errorf("%s: キー %q の書式は引数を %d 個使うが、%d 個渡している", call.pos, call.key, want, len(call.args))
		}
		for i, arg := range call.args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok {
				continue
			}
			for _, use := range formatArgs(t, entry.EN) {
				n, verb, _ := strings.Cut(use, ":")
				if n != strconv.Itoa(i+1) {
					continue
				}
				if (verb == "d") != (lit.Kind == token.INT) {
					t.Errorf("%s: キー %q の %d 番目の引数(%s)の型が、書式の %%%s と合わない", call.pos, call.key, i+1, lit.Value, verb)
				}
			}
		}
	}
	for key := range catalog {
		if !used[key] {
			t.Errorf("カタログのキー %q が、コードのどこでも使われていない(使われていない文言は、消す)", key)
		}
	}
	for _, pos := range direct {
		t.Errorf("%s: Message は Msg(key, args...) で作る(直接書くと、キーの検査をすり抜ける)", pos)
	}
}

func isMsgCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "Msg"
	case *ast.SelectorExpr:
		return f.Sel.Name == "Msg"
	}
	return false
}

func isMessageType(typ ast.Expr) bool {
	switch f := typ.(type) {
	case *ast.Ident:
		return f.Name == "Message"
	case *ast.SelectorExpr:
		return f.Sel.Name == "Message"
	}
	return false
}

func TestMessageTextFollowsTheLanguage(t *testing.T) {
	m := Msg(keyCommentTooLong, 2000)
	if got, want := m.Text(LangEN), "Comment is too long (maximum is 2000 characters)"; got != want {
		t.Errorf("英語 = %q, want %q", got, want)
	}
	if got, want := m.Text(LangJA), "コメントが長すぎます(最大 2000 文字)"; got != want {
		t.Errorf("日本語 = %q, want %q", got, want)
	}
	if got, want := m.Text(Lang("fr")), m.Text(LangEN); got != want {
		t.Errorf("未対応の言語 = %q, want 英語 %q", got, want)
	}
	if got := Msg("no.such.key").Text(LangJA); got != "no.such.key" {
		t.Errorf("カタログにないキー = %q, want キーそのもの", got)
	}
}

func TestValidationErrorTextsFollowTheLanguage(t *testing.T) {
	err := NewValidationError(Msg(keyReviewRatingRange, MinRating, MaxRating), Msg(keyCommentBlank))
	if want := []string{"Rating must be in 1..5", "Comment can't be blank"}; !reflect.DeepEqual(err.Texts(LangEN), want) {
		t.Errorf("Texts(en) = %v, want %v", err.Texts(LangEN), want)
	}
	if want := []string{"評価は 1〜5 の整数で指定してください", "コメントを入力してください"}; !reflect.DeepEqual(err.Texts(LangJA), want) {
		t.Errorf("Texts(ja) = %v, want %v", err.Texts(LangJA), want)
	}
	if want := "validation failed: Rating must be in 1..5, Comment can't be blank"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestValidatorsReturnJapaneseIssues(t *testing.T) {
	tests := []struct {
		name string
		got  []Message
		want []string
	}{
		{"パスワードが空", PasswordIssues(""), []string{"パスワードを入力してください"}},
		{"パスワードが短く、文字種も足りない", PasswordIssues("abc"), []string{"パスワードが短すぎます(最小 8 バイト)", "パスワードには、半角の英字・数字・記号を、それぞれ 1 文字以上含めてください"}},
		{"メールが空", EmailIssues(""), []string{"メールアドレスを入力してください"}},
		{"メールの形式が不正", EmailIssues("abc"), []string{"メールアドレスの形式が正しくありません"}},
		{"ユーザー名が空", UsernameIssues(""), []string{"ユーザー名を入力してください"}},
		{"自己紹介が長すぎる", BioIssues(strings.Repeat("あ", MaxBioChars+1)), []string{"自己紹介が長すぎます(最大 500 文字)"}},
		{"認証情報は、メール → パスワードの順に並ぶ", CredentialsIssues("", ""), []string{"メールアドレスを入力してください", "パスワードを入力してください"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Texts(LangJA, tt.got); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("日本語 = %v, want %v", got, tt.want)
			}
		})
	}
}
