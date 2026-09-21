package usecase_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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
			if !strings.HasSuffix(name, "s") {
				violations = append(violations, fmt.Sprintf("%s は名前が複数形(s で終わる)ではない (repository を持つ型は書き込みオブジェクトで、名前は集約の複数形にする。エンティティ・値オブジェクトは repository を持たない)", name))
			}
			want := repositoryNameFor(name)
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

// repositoryNameFor は、書き込みオブジェクトの名前（集約の複数形）から、持つべき repository の名前を返す。
// Shops なら ShopRepository、MailDeliveries（ies で終わる複数形）なら MailDeliveryRepository である。
func repositoryNameFor(writeObject string) string {
	if base, ok := strings.CutSuffix(writeObject, "ies"); ok {
		return base + "yRepository"
	}
	return strings.TrimSuffix(writeObject, "s") + "Repository"
}

// mailProviderImports は、メール送信のプロバイダー（SMTP など）の実装の詳細を表す import パスである。
// メールの形式・プロトコルを知るのは、腐敗防止層である adapter/infra だけで、usecase と domain は
// これらを import しない。
var mailProviderImports = []string{"net/smtp", "net/textproto", "mime", "crypto/tls"}

// mailProviderTerm は、プロバイダーの実装の用語（識別子に含まれると、usecase・domain が実装の詳細を
// 知っている印になる）である。"Resend" は、確認メールの「再送の間隔」という業務の言葉と重なるので含めない。
var mailProviderTerm = regexp.MustCompile(`(?i)smtp|mime|starttls|textproto|mailpit`)

// checkNoMailProviderDetail は、Go のソースが、メール送信のプロバイダーの実装の詳細を知らないことを
// 確かめ、違反の説明を返す。次の 3 つを検査する（コメントは対象外）。
//   - プロバイダーの実装の import（mailProviderImports）がない
//   - 識別子（型・関数・フィールド・変数の名前）に、プロバイダーの用語（mailProviderTerm）がない
//   - メールの件名を表すフィールド（Subject）を持つ型がない（メールの文面・書式は、送信側が決める）
func checkNoMailProviderDetail(src string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, bad := range mailProviderImports {
			if path == bad || strings.HasPrefix(path, bad+"/") {
				violations = append(violations, path+" を import している (メールの形式・プロトコルを知るのは adapter/infra だけ)")
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if mailProviderTerm.MatchString(x.Name) {
				violations = append(violations, x.Name+" はメール送信のプロバイダーの実装の用語を含む (実装の詳細は adapter/infra に閉じる)")
			}
		case *ast.StructType:
			for _, field := range x.Fields.List {
				for _, name := range field.Names {
					if name.Name == "Subject" {
						violations = append(violations, "Subject フィールドがある (メールの件名・本文を組み立てるのは adapter/infra で、usecase・domain は意図を表す値だけを渡す)")
					}
				}
			}
		}
		return true
	})
	slices.Sort(violations)
	return slices.Compact(violations), nil
}

// usedIdents は n の中で参照されている識別子の名前を返す。フィールド名・セレクタの右辺
// (x.Users の Users)・型や関数の宣言名は、参照ではないので数えない。
func usedIdents(n ast.Node) map[string]bool {
	used := map[string]bool{}
	var walk func(ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			used[x.Name] = true
		case *ast.SelectorExpr:
			ast.Inspect(x.X, walk)
			return false
		case *ast.Field:
			ast.Inspect(x.Type, walk)
			return false
		case *ast.TypeSpec:
			ast.Inspect(x.Type, walk)
			return false
		case *ast.FuncDecl:
			if x.Recv != nil {
				ast.Inspect(x.Recv, walk)
			}
			ast.Inspect(x.Type, walk)
			if x.Body != nil {
				ast.Inspect(x.Body, walk)
			}
			return false
		}
		return true
	}
	ast.Inspect(n, walk)
	return used
}

// checkEntitiesIndependentOfPersistence は、domain の永続化の宣言(repository の interface、
// 書き込みオブジェクトと Service、そのコンストラクタとメソッド、repository の interface のシグネチャ
// だけが使う型)以外の宣言、つまりエンティティ・値オブジェクト・規則が、永続化の宣言を参照して
// いないことを確かめ、違反の説明と、見つけた永続化の識別子(整列済み)を返す。
// domain は集約ごとに 1 ファイルにまとめているので、この境界はファイルにもパッケージにも
// 現れない。このテストで守る。
//
// repository の interface のシグネチャに現れる型のうち、永続化の宣言からしか参照されないもの
// (CreateUserParams など)は、永続化の側に数える。エンティティ(User など)は、規則の側からも
// 参照されるので、永続化の側には数えない。
func checkEntitiesIndependentOfPersistence(sources map[string]string) (violations, persistence []string, err error) {
	type decl struct {
		label   string
		defines []string
		recv    string
		node    ast.Node
	}
	var decls []decl
	typeNames := map[string]bool{}
	holders := map[string]bool{}
	persist := map[string]bool{}
	var signatureTypes []string
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		f, perr := parser.ParseFile(token.NewFileSet(), name, sources[name], 0)
		if perr != nil {
			return nil, nil, perr
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						typeNames[s.Name.Name] = true
						decls = append(decls, decl{label: s.Name.Name, defines: []string{s.Name.Name}, node: s})
						switch t := s.Type.(type) {
						case *ast.InterfaceType:
							if strings.HasSuffix(s.Name.Name, "Repository") {
								persist[s.Name.Name] = true
								for id := range usedIdents(t) {
									signatureTypes = append(signatureTypes, id)
								}
							}
						case *ast.StructType:
							holds := strings.HasSuffix(s.Name.Name, "Service")
							for _, field := range t.Fields.List {
								typ := field.Type
								if star, ok := typ.(*ast.StarExpr); ok {
									typ = star.X
								}
								if id, ok := typ.(*ast.Ident); ok && strings.HasSuffix(id.Name, "Repository") {
									holds = true
								}
							}
							if holds {
								holders[s.Name.Name] = true
								persist[s.Name.Name] = true
							}
						}
					case *ast.ValueSpec:
						var defines []string
						for _, id := range s.Names {
							defines = append(defines, id.Name)
						}
						decls = append(decls, decl{label: strings.Join(defines, ", "), defines: defines, node: s})
					}
				}
			case *ast.FuncDecl:
				dc := decl{label: d.Name.Name, node: d}
				if d.Recv != nil && len(d.Recv.List) == 1 {
					recv := d.Recv.List[0].Type
					if star, ok := recv.(*ast.StarExpr); ok {
						recv = star.X
					}
					if id, ok := recv.(*ast.Ident); ok {
						dc.recv = id.Name
						dc.label = id.Name + "." + d.Name.Name
					}
				} else {
					dc.defines = []string{d.Name.Name}
				}
				decls = append(decls, dc)
			}
		}
	}
	for h := range holders {
		persist["New"+h] = true
	}
	isPersistence := func(d decl) bool {
		if d.recv != "" {
			return persist[d.recv]
		}
		return len(d.defines) > 0 && slices.ContainsFunc(d.defines, func(n string) bool { return persist[n] })
	}
	for changed := true; changed; {
		changed = false
		for _, t := range signatureTypes {
			if persist[t] || !typeNames[t] {
				continue
			}
			only := true
			for _, d := range decls {
				if slices.Contains(d.defines, t) || !usedIdents(d.node)[t] {
					continue
				}
				if !isPersistence(d) {
					only = false
					break
				}
			}
			if only {
				persist[t] = true
				changed = true
			}
		}
	}
	for _, d := range decls {
		if isPersistence(d) {
			continue
		}
		used := usedIdents(d.node)
		for id := range used {
			if persist[id] {
				violations = append(violations, fmt.Sprintf("%s は永続化の型 %s を参照している (エンティティ・値オブジェクト・規則は、repository の interface と書き込みオブジェクトに依存しない)", d.label, id))
			}
		}
	}
	slices.Sort(violations)
	for id := range persist {
		persistence = append(persistence, id)
	}
	slices.Sort(persistence)
	return violations, persistence, nil
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
//   - domain は集約ごとに 1 ファイルで、エンティティ・repository の interface・書き込みオブジェクトが
//     同居する。エンティティ・値オブジェクト・規則は、repository の interface と書き込みオブジェクトを
//     参照しない(境界はファイルにもパッケージにも現れないので、このテストで守る)
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
		// 空振りで通らないよう、Shop / Review / User / SignupVerification / MailDelivery の
		// 5 つの Repository が見つかることも確かめる。
		if repositories < 5 {
			t.Errorf("Repository は %d 個しか見つからない (5 個以上を期待)", repositories)
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

	t.Run("実際の usecase と domain はメール送信のプロバイダーの実装の詳細を知らない", func(t *testing.T) {
		for _, dir := range []string{".", "../domain"} {
			for name, src := range productionSources(t, dir) {
				v, err := checkNoMailProviderDetail(src)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				for _, msg := range v {
					t.Errorf("%s: %s", name, msg)
				}
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
		// 空振りで通らないよう、Shop / Review / User / SignupVerification / MailDelivery の
		// 5 つの書き込みオブジェクトが見つかることも確かめる（MailDeliveries は ies で終わる複数形）。
		// Service は、複数の集約を跨ぐ更新が domain に入るまで 0 個でよい。
		if writeObjects < 5 {
			t.Errorf("書き込みオブジェクトは %d 個しか見つからない (5 個以上を期待)", writeObjects)
		}
	})

	t.Run("実際の domain のエンティティ・値オブジェクト・規則は永続化の型を参照しない", func(t *testing.T) {
		v, persistence, err := checkEntitiesIndependentOfPersistence(productionSources(t, "../domain"))
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range v {
			t.Error(msg)
		}
		// 空振りで通らないよう、永続化の側に数えるべき識別子が見つかることも確かめる。
		// CreateUserParams と ProfileChanges は、repository の interface のシグネチャだけが使う型である。
		for _, want := range []string{
			"ShopRepository", "ReviewRepository", "UserRepository",
			"Shops", "Reviews", "Users", "NewShops", "NewReviews", "NewUsers",
			"CreateUserParams", "ProfileChanges",
			"SignupVerificationRepository", "SignupVerifications", "SignupVerificationReceipt", "CreateSignupVerificationParams",
			"MailDeliveryRepository", "MailDeliveries", "NewMailDeliveries", "CreateMailDeliveryParams",
		} {
			if !slices.Contains(persistence, want) {
				t.Errorf("永続化の識別子に %s が見つからない (見つかったもの: %v)", want, persistence)
			}
		}
		for _, entity := range []string{"User", "Shop", "Review", "ShopReviewBurger", "SignupToken", "MailKind", "MailFailure"} {
			if slices.Contains(persistence, entity) {
				t.Errorf("エンティティ %s が永続化の側に数えられている", entity)
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

	mailProviderCases := []struct {
		name, src, want string
	}{
		{"意図を表す値だけなら違反なし", "package p\ntype SignupConfirmation struct{ To, ConfirmURL string }\ntype Mailer interface{ SendSignupConfirmation(SignupConfirmation) }", ""},
		{"業務の言葉の Resend(再送の間隔)は違反にしない", "package p\nconst SignupResendInterval = 60", ""},
		{"net/smtp の import を検出する", "package p\nimport \"net/smtp\"", "net/smtp を import している"},
		{"mime/multipart の import を検出する", "package p\nimport \"mime/multipart\"", "mime/multipart を import している"},
		{"crypto/tls の import を検出する", "package p\nimport \"crypto/tls\"", "crypto/tls を import している"},
		{"net/textproto の import を検出する", "package p\nimport \"net/textproto\"", "net/textproto を import している"},
		{"件名(Subject)のフィールドを持つ型を検出する", "package p\ntype Mail struct{ To, Subject, Body string }", "Subject フィールドがある"},
		{"SMTP の用語を含む識別子を検出する", "package p\ntype SMTPMailer struct{}", "SMTPMailer はメール送信のプロバイダーの実装の用語を含む"},
		{"STARTTLS の用語を含む識別子を検出する", "package p\nfunc useStartTLS() {}", "useStartTLS はメール送信のプロバイダーの実装の用語を含む"},
		{"MIME の用語を含む識別子を検出する", "package p\nvar mimeVersion = \"1.0\"", "mimeVersion はメール送信のプロバイダーの実装の用語を含む"},
		{"コメントの中の用語は対象外", "package p\n// SMTP で送るのは infra の責務である。\nconst X = 1", ""},
	}
	for _, tc := range mailProviderCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := checkNoMailProviderDetail(tc.src)
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
		{"エンティティ(単数形の名前)が repository を持てば検出する", "package p\ntype X struct{ repo XRepository }", "名前が複数形"},
		{"ies で終わる複数形は y の repository に対応づける(MailDeliveries は MailDeliveryRepository)", "package p\ntype MailDeliveries struct{ repo MailDeliveryRepository }", ""},
		{"ies で終わる複数形が、単純に s を除いた名前の repository を持てば検出する", "package p\ntype MailDeliveries struct{ repo MailDeliverieRepository }", "MailDeliveryRepository だけをちょうど 1 つ"},
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

	entityCases := []struct {
		name, src, want string
	}{
		{"永続化の宣言だけが参照する型と、エンティティの規則が別々なら違反なし", `package p
type X struct{ N int }
func (x X) Valid() bool { return x.N > 0 }
type XParams struct{ N int }
type XRepository interface{ CreateX(p XParams) (X, error) }
type Xs struct{ repo XRepository }
func NewXs(r XRepository) *Xs { return &Xs{repo: r} }
func (s *Xs) Create(p XParams) (X, error) { return s.repo.CreateX(p) }`, ""},
		{"エンティティのメソッドが repository を参照していれば検出する", `package p
type X struct{}
type XRepository interface{ CreateX() (X, error) }
func (x X) Save(r XRepository) {}`, "X.Save は永続化の型 XRepository"},
		{"規則の関数が書き込みオブジェクトを参照していれば検出する", `package p
type X struct{}
type XRepository interface{ CreateX() (X, error) }
type Xs struct{ repo XRepository }
func Validate(w *Xs) bool { return w != nil }`, "Validate は永続化の型 Xs"},
		{"規則の関数が書き込みオブジェクトのコンストラクタを呼べば検出する", `package p
type XRepository interface{ CreateX() }
type Xs struct{ repo XRepository }
func NewXs(r XRepository) *Xs { return &Xs{repo: r} }
func Helper() { _ = NewXs(nil) }`, "Helper は永続化の型 NewXs"},
		{"エンティティが永続化の形のパラメータ型を使っていれば、その型はエンティティ側になり違反なし", `package p
type XParams struct{ N int }
type X struct{ P XParams }
type XRepository interface{ CreateX(p XParams) (X, error) }`, ""},
		{"永続化の型と同じ名前のフィールドやセレクタは参照に数えない", `package p
type XRepository interface{ CreateX() }
type Xs struct{ repo XRepository }
type Y struct{ Xs int }
func (y Y) N() int { return y.Xs }`, ""},
		{"永続化の宣言が無ければ何も検出しない", "package p\ntype X struct{ N int }\nfunc (x X) Valid() bool { return x.N > 0 }", ""},
	}
	for _, tc := range entityCases {
		t.Run(tc.name, func(t *testing.T) {
			v, _, err := checkEntitiesIndependentOfPersistence(map[string]string{"src.go": tc.src})
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
