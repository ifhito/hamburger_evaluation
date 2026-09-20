package usecase_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// allowedPrefixes は、interface 名の接尾辞ごとに許されるメソッド名の接頭辞を表す。
var allowedPrefixes = map[string][]string{
	"Query":      {"Get", "List"},
	"Repository": {"Create", "Update", "Discard"},
}

// checkNaming は、Go のソースに含まれる *Query / *Repository interface のメソッド名を
// 規約と照らし、違反の説明と、接尾辞ごとに見つけた interface の数を返す。
func checkNaming(src string) (violations []string, found map[string]int, err error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	if err != nil {
		return nil, nil, err
	}
	found = map[string]int{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		it, ok := ts.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		for suffix, prefixes := range allowedPrefixes {
			if !strings.HasSuffix(ts.Name.Name, suffix) {
				continue
			}
			found[suffix]++
			for _, m := range it.Methods.List {
				if len(m.Names) == 0 {
					violations = append(violations, ts.Name.Name+" は interface を埋め込んでいる (Query / Repository では埋め込みを使わず、メソッドを直接宣言する)")
				}
				for _, id := range m.Names {
					if !slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(id.Name, p) }) {
						violations = append(violations, ts.Name.Name+"."+id.Name+" は "+strings.Join(prefixes, "/")+" で始まっていない")
					}
				}
			}
		}
		return true
	})
	return violations, found, nil
}

// checkNoRepositoryDependency は、Go のソースが *Repository の型を宣言せず、
// 別のパッケージの *Repository も参照していないことを確かめ、違反の説明を返す。
// usecase が repository に依存しないための検査である。import の別名(d "…/domain")で
// 回避されないよう、パッケージ名は問わず、名前の接尾辞だけで判定する。dot import は、
// 名前だけで参照できてしまい検査をすり抜けるため、使うこと自体を違反とする。
func checkNoRepositoryDependency(src string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, imp := range f.Imports {
		if imp.Name != nil && imp.Name.Name == "." {
			violations = append(violations, strings.Trim(imp.Path.Value, `"`)+" を dot import している (repository への依存を検査できなくなる)")
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if strings.HasSuffix(x.Name.Name, "Repository") {
				violations = append(violations, x.Name.Name+" を宣言している (repository の型は domain が持つ)")
			}
		case *ast.SelectorExpr:
			if strings.HasSuffix(x.Sel.Name, "Repository") {
				pkg := ""
				if id, ok := x.X.(*ast.Ident); ok {
					pkg = id.Name + "."
				}
				violations = append(violations, pkg+x.Sel.Name+" を参照している (usecase は repository に依存せず、書き込みは domain の書き込みオブジェクトを通す)")
			}
		}
		return true
	})
	return violations, nil
}

// checkStdlibOnly は、Go のソースが標準ライブラリ以外(import パスの先頭の要素に "." を含むもの。
// プロジェクト内のパッケージも第三者のパッケージも)を import していないことを確かめ、違反の説明を返す。
// domain が標準ライブラリだけに依存するための検査である。
func checkStdlibOnly(src string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
			violations = append(violations, path+" を import している (domain は標準ライブラリだけに依存する)")
		}
	}
	return violations, nil
}

// checkImports は、Go のソースが forbidden のいずれかを含む import パスを持たないことを
// 確かめ、違反の説明を返す。
func checkImports(src string, forbidden []string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, bad := range forbidden {
			if strings.Contains(path, bad) {
				violations = append(violations, path+" を import している (内向きの依存だけが許される)")
			}
		}
	}
	return violations, nil
}

// maxWriteMethods は、domain の書き込みオブジェクトと Service の、1 つの型あたりの公開
// メソッド数の上限である。肥大化を防ぐ歯止めで、超えるときは、ロジックをエンティティへ
// 移す、集約を分ける、を先に検討する(domain/doc.go のルール)。上限を上げるときは、
// 理由をレビューで示す。
const maxWriteMethods = 8

// checkWriteObjectRules は、domain で repository を持つ型(*Repository 型のフィールドを持つ
// struct と、名前が Service で終わる struct)が、ルールに沿っていることを確かめ、違反の説明と、
// 見つけた書き込みオブジェクトと Service の数を返す。ルールは次のとおり。
//   - 書き込みオブジェクト(Service ではない型): 自分の集約の repository をちょうど 1 つだけ持つ。
//     名前は集約の複数形で、Shops なら ShopRepository である
//   - Service(名前が Service で終わる型): 2 種類以上の repository だけを持つ。複数の集約を
//     跨ぐ更新のためのもので、1 種類だけなら、その集約の書き込みオブジェクトに置く
//   - どちらも、公開メソッドが maxWriteMethods 個まで
func checkWriteObjectRules(sources map[string]string) (violations []string, writeObjects, services int, err error) {
	type holder struct {
		fields int
		others int
		repos  []string
	}
	holders := map[string]*holder{}
	methods := map[string]int{}
	for name, src := range sources {
		f, perr := parser.ParseFile(token.NewFileSet(), name, src, 0)
		if perr != nil {
			return nil, 0, 0, perr
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					h := &holder{}
					for _, field := range st.Fields.List {
						n := max(1, len(field.Names))
						h.fields += n
						typ := field.Type
						if star, ok := typ.(*ast.StarExpr); ok {
							typ = star.X
						}
						if id, ok := typ.(*ast.Ident); ok && strings.HasSuffix(id.Name, "Repository") {
							if !slices.Contains(h.repos, id.Name) {
								h.repos = append(h.repos, id.Name)
							}
						} else {
							h.others += n
						}
					}
					if len(h.repos) > 0 || strings.HasSuffix(ts.Name.Name, "Service") {
						holders[ts.Name.Name] = h
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) != 1 || !d.Name.IsExported() {
					continue
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if id, ok := recv.(*ast.Ident); ok {
					methods[id.Name]++
				}
			}
		}
	}
	names := make([]string, 0, len(holders))
	for name := range holders {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		h := holders[name]
		if strings.HasSuffix(name, "Service") {
			services++
			if len(h.repos) < 2 {
				violations = append(violations, fmt.Sprintf("%s は repository を %d 種類しか持たない (Service は複数の集約を跨ぐ更新だけに使う。1 つの集約だけを更新するなら、その集約の書き込みオブジェクトに置く)", name, len(h.repos)))
			}
			if h.others > 0 {
				violations = append(violations, fmt.Sprintf("%s は repository 以外のフィールドを持っている (Service が持つのは repository だけ)", name))
			}
		} else {
			writeObjects++
			want := strings.TrimSuffix(name, "s") + "Repository"
			if h.fields != 1 || len(h.repos) != 1 || h.repos[0] != want {
				violations = append(violations, fmt.Sprintf("%s は %s だけをちょうど 1 つ持たなければならない (書き込みオブジェクトは自分の集約の repository だけを持つ。他の集約に触れる手順は Service に置く)", name, want))
			}
		}
		if methods[name] > maxWriteMethods {
			violations = append(violations, fmt.Sprintf("%s の公開メソッドが %d 個ある (上限 %d 個。エンティティへの移動か集約の分割を先に検討する)", name, methods[name], maxWriteMethods))
		}
	}
	return violations, writeObjects, services, nil
}

// checkNoRepositoryImport は、Go のソースが repository の実装パッケージ(adapter/repository。
// その下の sqlcgen は含まない)を import していないことを確かめ、違反の説明を返す。
func checkNoRepositoryImport(src string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasSuffix(path, "/internal/adapter/repository") {
			violations = append(violations, path+" を import している (repository の実装を呼ぶのは組み立て(cmd)だけ)")
		}
	}
	return violations, nil
}

// productionSources は、dir にあるテスト以外の Go ファイルの (ファイル名, 内容) を返す。
func productionSources(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sources[filepath.Join(dir, e.Name())] = string(src)
	}
	return sources
}

// TestPersistenceInterfaceNaming は、永続化の依存の規約を固定する。
//   - usecase は読み取りを *Query (Get*/List*) で行い、*Repository を宣言も参照もしない
//   - repository の interface (*Repository。Create*/Update*/Discard*) は domain が宣言し、
//     呼ぶのは domain のコードだけである(単一の集約の書き込みは集約ごとの書き込みオブジェクト、
//     複数の集約を跨ぐ更新だけが Service)
//   - usecase と domain は、adapter・HTTP・SQL ドライバを import しない。
//     domain はさらに、標準ライブラリ以外(第三者・プロジェクト内)を import しない
func TestPersistenceInterfaceNaming(t *testing.T) {
	t.Run("実際の usecase パッケージが規約に沿っている", func(t *testing.T) {
		queries := 0
		for name, src := range productionSources(t, ".") {
			v, found, err := checkNaming(src)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", name, msg)
			}
			queries += found["Query"]
			if found["Repository"] > 0 {
				t.Errorf("%s: usecase が *Repository の interface を宣言している (domain が宣言する)", name)
			}
		}
		// 空振りで通らないよう、Shop / Review / User の 3 つの Query が見つかることも確かめる。
		if queries < 3 {
			t.Errorf("Query は %d 個しか見つからない (3 個以上を期待)", queries)
		}
	})

	t.Run("実際の domain パッケージの Repository が規約に沿っている", func(t *testing.T) {
		repositories := 0
		for name, src := range productionSources(t, "../domain") {
			v, found, err := checkNaming(src)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", name, msg)
			}
			repositories += found["Repository"]
		}
		// 空振りで通らないよう、Shop / Review / User の 3 つの Repository が見つかることも確かめる。
		if repositories < 3 {
			t.Errorf("Repository は %d 個しか見つからない (3 個以上を期待)", repositories)
		}
	})

	t.Run("実際の usecase は repository に依存しない", func(t *testing.T) {
		for name, src := range productionSources(t, ".") {
			v, err := checkNoRepositoryDependency(src)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", name, msg)
			}
		}
	})

	t.Run("実際の usecase と domain は外向きのパッケージを import しない", func(t *testing.T) {
		outward := []string{"net/http", "database/sql", "pgx", "/internal/adapter/"}
		for name, src := range productionSources(t, ".") {
			v, err := checkImports(src, outward)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", name, msg)
			}
		}
		// domain は usecase と photo も import しない (標準ライブラリだけに依存する)。
		domainOutward := append([]string{"/internal/usecase", "/internal/photo"}, outward...)
		for name, src := range productionSources(t, "../domain") {
			v, err := checkImports(src, domainOutward)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", name, msg)
			}
		}
	})

	t.Run("実際の domain は標準ライブラリだけを import する", func(t *testing.T) {
		for name, src := range productionSources(t, "../domain") {
			v, err := checkStdlibOnly(src)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", name, msg)
			}
		}
	})

	t.Run("実際の handler と query は repository に依存しない", func(t *testing.T) {
		for _, dir := range []string{"../adapter/handler", "../adapter/query"} {
			for name, src := range productionSources(t, dir) {
				v, err := checkNoRepositoryDependency(src)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				imports, err := checkNoRepositoryImport(src)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				for _, msg := range append(v, imports...) {
					t.Errorf("%s: %s", name, msg)
				}
			}
		}
	})

	t.Run("実際の domain の書き込みオブジェクトと Service がルールに沿っている", func(t *testing.T) {
		v, writeObjects, _, err := checkWriteObjectRules(productionSources(t, "../domain"))
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range v {
			t.Error(msg)
		}
		// 空振りで通らないよう、Shop / Review / User の 3 つの書き込みオブジェクトが見つかることも確かめる。
		// Service は、複数の集約を跨ぐ更新が domain に入るまで 0 個でよい。
		if writeObjects < 3 {
			t.Errorf("書き込みオブジェクトは %d 個しか見つからない (3 個以上を期待)", writeObjects)
		}
	})

	cases := []struct {
		name, src, want string // want は期待する違反の説明の一部 (空なら違反なし)
	}{
		{"規約どおりなら違反なし", "package p\ntype XQuery interface{ GetX(); ListX() }\ntype XRepository interface{ CreateX(); UpdateX(); DiscardX() }", ""},
		{"Repository に読み取りがあれば検出する", "package p\ntype XRepository interface{ CreateX(); GetX() }", "XRepository.GetX"},
		{"Query に書き込みがあれば検出する", "package p\ntype XQuery interface{ GetX(); CreateX() }", "XQuery.CreateX"},
		{"埋め込み interface は検出する", "package p\ntype XQuery interface{ GetX() }\ntype XRepository interface{ XQuery; CreateX() }", "XRepository"},
		{"対象外の interface は無視する", "package p\ntype PasswordHasher interface{ Hash() }", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, _, err := checkNaming(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(v, "\n"); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("違反 = %q, 期待 = %q", got, tc.want)
			}
		})
	}

	dependencyCases := []struct {
		name, src, want string
	}{
		{"usecase が Repository の interface を宣言していれば検出する", "package p\ntype XRepository interface{ CreateX() }", "XRepository を宣言している"},
		{"usecase が domain.XRepository を参照していれば検出する", "package p\nimport \"domain\"\ntype S struct{ repo domain.XRepository }", "domain.XRepository を参照している"},
		{"domain の書き込みオブジェクトの参照は許す", "package p\nimport \"domain\"\ntype S struct{ writes *domain.Xs }", ""},
		{"別名の import でも XRepository の参照を検出する", "package p\nimport d \"domain\"\ntype S struct{ repo d.XRepository }", "d.XRepository を参照している"},
		{"struct で Repository を宣言していても検出する", "package p\ntype XRepository struct{}", "XRepository を宣言している"},
		{"dot import を検出する", "package p\nimport . \"domain\"", "dot import している"},
	}
	for _, tc := range dependencyCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := checkNoRepositoryDependency(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(v, "\n"); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("違反 = %q, 期待 = %q", got, tc.want)
			}
		})
	}

	writeObjectSrc := func(methods int) string {
		s := "package p\ntype Xs struct{ repo XRepository }\n"
		for i := 0; i < methods; i++ {
			s += fmt.Sprintf("func (s *Xs) M%d() {}\n", i)
		}
		return s
	}
	writeObjectCases := []struct {
		name, src, want string
	}{
		{"書き込みオブジェクトが自分の repository を 1 つだけ持てば違反なし", writeObjectSrc(1), ""},
		{"公開メソッドが上限ちょうどなら違反なし", writeObjectSrc(maxWriteMethods), ""},
		{"公開メソッドが上限を超えれば検出する", writeObjectSrc(maxWriteMethods + 1), "公開メソッドが"},
		{"書き込みオブジェクトが別の集約の repository を持てば検出する", "package p\ntype Xs struct{ repo YRepository }", "XRepository だけをちょうど 1 つ"},
		{"書き込みオブジェクトが repository を 2 種類持てば検出する", "package p\ntype Xs struct {\n\trepo XRepository\n\tother YRepository\n}", "XRepository だけをちょうど 1 つ"},
		{"書き込みオブジェクトが repository 以外を持てば検出する", "package p\ntype Xs struct {\n\trepo XRepository\n\tn int\n}", "XRepository だけをちょうど 1 つ"},
		{"1 種類の repository だけを持つ Service を検出する", "package p\ntype XService struct{ repo XRepository }", "Service は複数の集約を跨ぐ更新だけに使う"},
		{"repository を持たない Service も検出する", "package p\ntype XService struct{}", "repository を 0 種類しか持たない"},
		{"同じ repository を 2 フィールドで持つ Service も 1 種類として検出する", "package p\ntype XService struct {\n\ta XRepository\n\tb XRepository\n}", "repository を 1 種類しか持たない"},
		{"2 種類の repository を持つ Service は違反なし", "package p\ntype XYService struct {\n\tx XRepository\n\ty YRepository\n}", ""},
		{"Service が repository 以外を持てば検出する", "package p\ntype XYService struct {\n\tx XRepository\n\ty YRepository\n\tn int\n}", "repository 以外のフィールド"},
		{"repository を持たない通常の struct は対象外", "package p\ntype Shop struct{ Name string }", ""},
	}
	for _, tc := range writeObjectCases {
		t.Run(tc.name, func(t *testing.T) {
			v, _, _, err := checkWriteObjectRules(map[string]string{"src.go": tc.src})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(v, "\n"); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("違反 = %q, 期待 = %q", got, tc.want)
			}
		})
	}

	repositoryImportCases := []struct {
		name, src, want string
	}{
		{"repository の実装パッケージの import を検出する", "package p\nimport \"example.com/x/internal/adapter/repository\"", "adapter/repository"},
		{"sqlcgen の import は許す", "package p\nimport \"example.com/x/internal/adapter/repository/sqlcgen\"", ""},
	}
	for _, tc := range repositoryImportCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := checkNoRepositoryImport(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(v, "\n"); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("違反 = %q, 期待 = %q", got, tc.want)
			}
		})
	}

	stdlibCases := []struct {
		name, src, want string
	}{
		{"第三者のパッケージを検出する", "package p\nimport \"golang.org/x/text\"", "golang.org/x/text"},
		{"プロジェクト内のパッケージを検出する", "package p\nimport \"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase\"", "internal/usecase"},
		{"標準ライブラリだけなら違反なし", "package p\nimport (\n\t\"context\"\n\t\"net/url\"\n)", ""},
	}
	for _, tc := range stdlibCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := checkStdlibOnly(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(v, "\n"); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("違反 = %q, 期待 = %q", got, tc.want)
			}
		})
	}

	importCases := []struct {
		name, src, want string
	}{
		{"外向きの import を検出する", "package p\nimport \"database/sql\"", "database/sql"},
		{"標準ライブラリだけなら違反なし", "package p\nimport \"context\"", ""},
	}
	for _, tc := range importCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := checkImports(tc.src, []string{"net/http", "database/sql", "pgx"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(v, "\n"); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("違反 = %q, 期待 = %q", got, tc.want)
			}
		})
	}
}
