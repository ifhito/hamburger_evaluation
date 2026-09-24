package handler_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// exposedInMeta は、GET /meta の応答に出している domain の上限・下限の定数である。
// 応答の値が domain の定数と一致することは、TestMeta が確かめる。
var exposedInMeta = []string{
	"MinRating", "MaxRating",
	"MaxCommentChars", "MaxBurgerNameChars", "MaxShopNameChars",
	"MaxUsernameChars", "MaxBioChars", "MaxModerationNoteChars",
	"MinPasswordBytes", "MaxPasswordBytes",
	"MaxPhotoBytes", "MaxPhotoEdge",
}

// notExposedInMeta は、GET /meta に出さない上限・下限の定数と、出さない理由である。
// frontend が入力欄に見せる必要のない値だけをここに置く。見せる必要があるなら、GET /meta に足す。
var notExposedInMeta = map[string]string{
	"MaxEmailChars":               "メールアドレスの入力欄には文字数のカウンターを付けない。長すぎるときは、backend の 422 の文言で伝える",
	"MaxRecalcFailureReasonChars": "統計の再計算の失敗の理由を、切り詰めて保存するための内部の上限で、利用者の入力欄ではない",
	"MaxMailErrorLength":          "メール送信の失敗の記録を、切り詰めて保存するための内部の上限で、利用者の入力欄ではない",
	"MaxOAuthClientNameLength":    "外部のアプリが自分で知らせる名前の検査で、利用者の入力欄ではない",
	"MaxOAuthRedirectURIs":        "外部のアプリが自分で知らせる戻り先の数の検査で、利用者の入力欄ではない",
	"MaxOAuthURILength":           "外部のアプリが自分で知らせる URL の長さの検査で、利用者の入力欄ではない",
	"MaxClientMetadataBytes":      "外部のアプリの説明ファイルを取り込むときの大きさの検査で、利用者の入力欄ではない",
	"MaxPerPage":                  "一覧の 1 ページの件数の上限。frontend は per_page を送らず、続きがあるかを X-Has-More で知るだけなので、値を知る必要がない",
	"MaxMapURLChars":              "地図リンクの入力欄には文字数のカウンターを付けない。長すぎるときは、backend の 422 の文言で伝える",
}

// domainLimitConstants は、domain パッケージ(テストを除く)で宣言された、Max / Min で始まる
// 公開の定数の名前を返す。
func domainLimitConstants(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../domain/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("domain のソースが見つからない: %v", err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("%s を読めない: %v", file, err)
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				for _, id := range spec.(*ast.ValueSpec).Names {
					if id.IsExported() && (strings.HasPrefix(id.Name, "Max") || strings.HasPrefix(id.Name, "Min")) {
						names = append(names, id.Name)
					}
				}
			}
		}
	}
	slices.Sort(names)
	return names
}

// TestDomainLimitsAreExposedInMetaOrExplained は、domain に上限・下限の定数を足したのに、
// GET /meta に出し忘れることを防ぐ。定数は、応答に出しているか、出さない理由が書かれているかの
// どちらかでなければならない。定数を消したのに一覧に残った古い項目も、失敗にする。
func TestDomainLimitsAreExposedInMetaOrExplained(t *testing.T) {
	declared := domainLimitConstants(t)

	for _, name := range declared {
		exposed := slices.Contains(exposedInMeta, name)
		reason, explained := notExposedInMeta[name]
		switch {
		case exposed && explained:
			t.Errorf("%s は、GET /meta に出す一覧と、出さない一覧の両方にある。どちらかにする", name)
		case !exposed && !explained:
			t.Errorf("%s は domain の上限・下限の定数だが、GET /meta に出しておらず、出さない理由もない。"+
				"入力欄に見せる値なら GET /meta に足し、そうでなければ notExposedInMeta に理由を書く", name)
		case explained && strings.TrimSpace(reason) == "":
			t.Errorf("%s を GET /meta に出さない理由が空になっている", name)
		}
	}

	for _, name := range exposedInMeta {
		if !slices.Contains(declared, name) {
			t.Errorf("exposedInMeta の %s は、domain に定数がない(消した、または名前を変えた)。一覧を直す", name)
		}
	}
	for name := range notExposedInMeta {
		if !slices.Contains(declared, name) {
			t.Errorf("notExposedInMeta の %s は、domain に定数がない(消した、または名前を変えた)。一覧を直す", name)
		}
	}
}
