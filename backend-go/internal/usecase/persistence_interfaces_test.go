package usecase_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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

// TestPersistenceInterfaceNaming は、永続化 interface の命名規約
// (Query は Get*/List*、Repository は Create*/Update*/Discard* のみ) を固定する。
func TestPersistenceInterfaceNaming(t *testing.T) {
	t.Run("実際の usecase パッケージが規約に沿っている", func(t *testing.T) {
		entries, err := os.ReadDir(".")
		if err != nil {
			t.Fatal(err)
		}
		total := map[string]int{}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			src, err := os.ReadFile(e.Name())
			if err != nil {
				t.Fatal(err)
			}
			v, found, err := checkNaming(string(src))
			if err != nil {
				t.Fatalf("%s: %v", e.Name(), err)
			}
			for _, msg := range v {
				t.Errorf("%s: %s", e.Name(), msg)
			}
			for k, n := range found {
				total[k] += n
			}
		}
		// 空振りで通らないよう、Shop / Review / User の 3 組が見つかることも確かめる。
		for suffix := range allowedPrefixes {
			if total[suffix] < 3 {
				t.Errorf("%s は %d 個しか見つからない (3 個以上を期待)", suffix, total[suffix])
			}
		}
	})

	cases := []struct {
		name, src, want string // want は期待する違反の説明の一部 (空なら違反なし)
	}{
		{"規約どおりなら違反なし", "package p\ntype XQuery interface{ GetX(); ListX() }\ntype XRepository interface{ CreateX(); UpdateX(); DiscardX() }", ""},
		{"Repository に読み取りがあれば検出する", "package p\ntype XRepository interface{ CreateX(); GetX() }", "XRepository.GetX"},
		{"Query に書き込みがあれば検出する", "package p\ntype XQuery interface{ GetX(); CreateX() }", "XQuery.CreateX"},
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
}
