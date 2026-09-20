package usecase_test

import (
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

// checkNoRepositoryDependency は、Go のソースが *Repository の interface を宣言せず、
// domain.*Repository も参照していないことを確かめ、違反の説明を返す。usecase が
// repository に依存しないための検査である。
func checkNoRepositoryDependency(src string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	if err != nil {
		return nil, err
	}
	var violations []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if _, ok := x.Type.(*ast.InterfaceType); ok && strings.HasSuffix(x.Name.Name, "Repository") {
				violations = append(violations, x.Name.Name+" を宣言している (repository の interface は domain が宣言する)")
			}
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "domain" && strings.HasSuffix(x.Sel.Name, "Repository") {
				violations = append(violations, "domain."+x.Sel.Name+" を参照している (usecase は repository に依存せず、書き込みは domain のサービスを通す)")
			}
		}
		return true
	})
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
//     呼ぶのは domain のサービスだけである
//   - usecase と domain は、adapter・HTTP・SQL ドライバを import しない
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
		{"domain のサービスの参照は許す", "package p\nimport \"domain\"\ntype S struct{ svc *domain.XService }", ""},
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
