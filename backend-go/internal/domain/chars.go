package domain

import (
	"fmt"
	"unicode/utf8"
)

// 文字数の上限そのものは、ルールを持つ側(review.go・shop.go・username.go・email.go)に、
// 検証の関数と並べて置く。ここには、上限の判定に共通の道具だけを置く。
// 文字数は Unicode のコードポイント数で数える（バイト数でも、書記素クラスタでもない。
// 日本語は 1 文字 = 1、通常の絵文字も 1）。DB の char_length も同じ数え方である。

// exceedsChars は s のコードポイント数が max を超えるかを返す。
func exceedsChars(s string, max int) bool {
	return utf8.RuneCountInString(s) > max
}

// tooLongMessage は上限超過の Rails 形式の full message を返す。メッセージは
// API の外部契約なので英語のままである（password の "is too long" と同じ形）。
func tooLongMessage(label string, max int) string {
	return fmt.Sprintf("%s is too long (maximum is %d characters)", label, max)
}
