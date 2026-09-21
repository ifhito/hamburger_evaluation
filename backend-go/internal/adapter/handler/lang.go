package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// maxLanguageRanges は、Accept-Language から読む言語の指定の数の上限である。実際のブラウザは数個しか
// 送らないので、巨大なヘッダーで処理を増やされないための上限である(上限を超えた分は、分割もしない)。
const maxLanguageRanges = 16

// accepted は、ある言語について、指定された最高の優先度(q)と、それが書かれた位置である。
type accepted struct {
	q   float64
	at  int
	set bool
}

func (a *accepted) offer(q float64, at int) {
	if !a.set || q > a.q {
		*a = accepted{q: q, at: at, set: true}
	}
}

// langOf は、request の Accept-Language から、応答の文言の言語を決める。対応するのは日本語(ja)と
// 英語(en)で、ja-JP・ja_JP・en-US のような地域つきの指定は、言語の部分で判断する。優先度(q)の高い順に
// 選び、同じ優先度なら、書かれた順である。q=0 の言語は選ばない。`*` は、明示のない言語すべての優先度なので、
// たとえば `en;q=0, *;q=0.5` は日本語になる。指定がない・対応する言語がない(fr など)・読めない指定だけの
// ときは、英語(いまと同じ)を返す。複数行に分かれた Accept-Language は、コンマでつないだ 1 行として読む。
// 言語を選ぶのは HTTP の入り口の仕事で、domain は、選ばれた言語で文字列にするだけである。
func langOf(r *http.Request) domain.Lang {
	var ja, en, star accepted
	header := strings.Join(r.Header.Values("Accept-Language"), ",")
	for i, part := range strings.SplitN(header, ",", maxLanguageRanges+1) {
		if i >= maxLanguageRanges {
			break
		}
		tag, q, ok := parseLanguageRange(part)
		if !ok {
			continue
		}
		switch primarySubtag(tag) {
		case "ja":
			ja.offer(q, i)
		case "en":
			en.offer(q, i)
		case "*":
			star.offer(q, i)
		}
	}

	best, bestQ, bestAt := domain.LangEN, 0.0, 0
	for _, c := range []struct {
		lang domain.Lang
		a    accepted
	}{{domain.LangEN, en}, {domain.LangJA, ja}} {
		a := c.a
		if !a.set {
			a = star
		}
		if a.set && a.q > 0 && (a.q > bestQ || a.q == bestQ && a.at < bestAt) {
			best, bestQ, bestAt = c.lang, a.q, a.at
		}
	}
	return best
}

// parseLanguageRange は、`ja-JP;q=0.8` のような 1 つの指定を、言語のタグと優先度に分ける。q がなければ 1 である。
// 読めない指定(空・q が 0〜1 の 10 進の数でない)は、ok=false を返す。
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
		value = strings.TrimSpace(value)
		// ParseFloat は NaN・Inf・16 進・指数も読むので、q の書式(数字と小数点だけ)に先に絞る。
		if value == "" || strings.Trim(value, "0123456789.") != "" {
			return "", 0, false
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return "", 0, false
		}
		q = parsed
	}
	return tag, q, true
}

// primarySubtag は、言語のタグの最初の部分(ja-JP・ja_JP なら ja)を返す。
func primarySubtag(tag string) string {
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		return tag[:i]
	}
	return tag
}
