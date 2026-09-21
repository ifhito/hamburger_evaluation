package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// maxLanguageRanges は、Accept-Language から読む言語の指定の数の上限である。実際のブラウザは数個しか
// 送らないので、巨大なヘッダーで処理を増やされないための上限である。
const maxLanguageRanges = 16

// langOf は、request の Accept-Language から、応答の文言の言語を決める。対応するのは日本語(ja)と
// 英語(en)で、ja-JP・en-US のような地域つきの指定は、言語の部分で判断する。優先度(q)の高い順に
// 選び、同じ優先度なら、書かれた順である。q=0 の言語は選ばない。`*` は、指定のないすべての言語なので、
// 既定の英語として数える。指定がない・対応する言語がない(fr など)・読めない指定だけのときは、英語(いまと
// 同じ)を返す。言語を選ぶのは HTTP の入り口の仕事で、domain は、選ばれた言語で文字列にするだけである。
func langOf(r *http.Request) domain.Lang {
	best, bestQ := domain.LangEN, 0.0
	found := false
	for i, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		if i >= maxLanguageRanges {
			break
		}
		tag, q, ok := parseLanguageRange(part)
		if !ok || q <= 0 {
			continue
		}
		var lang domain.Lang
		switch primarySubtag(tag) {
		case "ja":
			lang = domain.LangJA
		case "en", "*":
			lang = domain.LangEN
		default:
			continue
		}
		if !found || q > bestQ {
			best, bestQ, found = lang, q, true
		}
	}
	return best
}

// parseLanguageRange は、`ja-JP;q=0.8` のような 1 つの指定を、言語のタグと優先度に分ける。q がなければ 1 である。
// 読めない指定(空・q が 0〜1 の数でない)は、ok=false を返す。
func parseLanguageRange(part string) (tag string, q float64, ok bool) {
	fields := strings.Split(part, ";")
	tag = strings.ToLower(strings.TrimSpace(fields[0]))
	if tag == "" {
		return "", 0, false
	}
	q = 1
	for _, param := range fields[1:] {
		name, value, found := strings.Cut(strings.TrimSpace(param), "=")
		if !found || strings.ToLower(strings.TrimSpace(name)) != "q" {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return "", 0, false
		}
		q = parsed
	}
	return tag, q, true
}

// primarySubtag は、言語のタグの最初の部分(ja-JP なら ja)を返す。
func primarySubtag(tag string) string {
	primary, _, _ := strings.Cut(tag, "-")
	return primary
}
