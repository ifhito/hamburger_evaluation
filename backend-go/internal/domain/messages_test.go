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
	for key, entry := range Catalog {
		if strings.TrimSpace(entry.EN) == "" {
			t.Errorf("キー %q の英語の文言が空である", key)
		}
		if strings.TrimSpace(entry.JA) == "" {
			t.Errorf("キー %q の日本語の文言が空である(キーを足したら、英語と日本語の両方を書く)", key)
		}
	}
}

func TestCatalogEntriesUseTheSameValuesInBothLanguages(t *testing.T) {
	for key, entry := range Catalog {
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
		if _, ok := Catalog[value]; !ok {
			t.Errorf("定数 %s のキー %q が、カタログにない", name, value)
		}
	}
	for key := range Catalog {
		if !values[key] {
			t.Errorf("カタログのキー %q に対応する `key…` の定数がない(使われていない文言は、消す)", key)
		}
	}
}

// TestMessagesInTheCodeAreInTheCatalog は、domain と usecase のコードが Msg(...) で作る文言のキーが、
// カタログにあることを確かめる。キーは、文字列のリテラルか、messages.go の `key…` の定数だけを許す。
func TestMessagesInTheCodeAreInTheCatalog(t *testing.T) {
	consts := keyConstants(t)
	found := 0
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
					call, ok := n.(*ast.CallExpr)
					if !ok || len(call.Args) == 0 || !isMsgCall(call.Fun) {
						return true
					}
					found++
					var key string
					switch arg := call.Args[0].(type) {
					case *ast.BasicLit:
						key, _ = strconv.Unquote(arg.Value)
					case *ast.Ident:
						key = consts[arg.Name]
					}
					if _, ok := Catalog[key]; !ok {
						t.Errorf("%s: Msg のキーが、カタログにない(またはキーを、リテラル・key… の定数で書いていない): %s", fset.Position(call.Pos()), path)
					}
					return true
				})
			}
		}
	}
	if found == 0 {
		t.Fatal("Msg(...) の呼び出しが 1 つも見つからない(検査が空振りしている)")
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
	if want := []string{"Rating must be in 1..5", "Comment can't be blank"}; !reflect.DeepEqual(err.Messages, want) {
		t.Errorf("Messages(従来の英語) = %v, want %v", err.Messages, want)
	}
	if want := []string{"評価は 1〜5 の整数で入力してください", "コメントを入力してください"}; !reflect.DeepEqual(err.Texts(LangJA), want) {
		t.Errorf("Texts(ja) = %v, want %v", err.Texts(LangJA), want)
	}
	if !reflect.DeepEqual(err.Texts(LangEN), err.Messages) {
		t.Errorf("Texts(en) = %v, want Messages %v", err.Texts(LangEN), err.Messages)
	}
	legacy := &ValidationError{Messages: []string{"Legacy message"}}
	if got := legacy.Texts(LangJA); !reflect.DeepEqual(got, legacy.Messages) {
		t.Errorf("Items のない ValidationError の Texts(ja) = %v, want 英語の Messages %v", got, legacy.Messages)
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
