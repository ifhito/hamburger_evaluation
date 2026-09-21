package domain

import "fmt"

// Lang は、利用者に返す文言の言語である。既定は英語(LangEN。いまの API の文言の契約)で、
// 言語の決定(Accept-Language の解釈)は、HTTP の入り口(handler)の仕事である。domain は、言語を選ばず、
// 文言のカタログを持つだけである。
type Lang string

const (
	LangEN Lang = "en"
	LangJA Lang = "ja"
)

// Entry は、1 つの文言の、言語ごとの書式(fmt の書式。値が入る所は %d・%s。日本語で値の順序が
// 変わるときは %[2]d のように番号で指す)である。
type Entry struct {
	EN string
	JA string
}

// Format は、l の言語の書式に args を入れた文字列を返す。日本語の書式が空のときは英語を使う(通常は、
// 構造テストが空を検出するので、起こらない)。
func (e Entry) Format(l Lang, args ...any) string {
	format := e.EN
	if l == LangJA && e.JA != "" {
		format = e.JA
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// Message は、利用者に見える文言を、言語に依らない形(キー + 引数)で表す。文字列にするのは、
// 言語を知っている側(handler)で、カタログ(Catalog)の書式に引数を入れる。
type Message struct {
	Key  string
	Args []any
}

// Msg は、キーと引数から Message を作る。
func Msg(key string, args ...any) Message {
	return Message{Key: key, Args: args}
}

// Text は、この domain の文言のカタログ(Catalog)で、l の言語の文字列にする。カタログにないキーは、
// キーをそのまま返す(構造テストが、カタログにないキーを検出する)。
func (m Message) Text(l Lang) string {
	return Render(Catalog, l, m)
}

// Render は、カタログ catalog で m を l の言語の文字列にする。domain の外の層(handler)も、自分の
// カタログを、同じ仕組みで文字列にするために使う。カタログにないキーは、キーをそのまま返す。
func Render(catalog map[string]Entry, l Lang, m Message) string {
	entry, ok := catalog[m.Key]
	if !ok {
		return m.Key
	}
	return entry.Format(l, m.Args...)
}

// Texts は、ms を l の言語の文字列にして並べる。ms が空なら nil を返す。
func Texts(l Lang, ms []Message) []string {
	if len(ms) == 0 {
		return nil
	}
	texts := make([]string, len(ms))
	for i, m := range ms {
		texts[i] = m.Text(l)
	}
	return texts
}
