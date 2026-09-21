package domain

import (
	"fmt"
	"unicode/utf8"
)

// テキスト入力の文字数の上限。文字数は Unicode のコードポイント数で数える
// （バイト数でも、書記素クラスタでもない。日本語は 1 文字 = 1、通常の絵文字も 1）。
// この値は DB の CHECK 制約（マイグレーション 000009。char_length も
// コードポイント数）と同じでなければならない。食い違いは db/migrations_test.go が検出する。
// 上限を変えるときは、この定数と、新しいマイグレーションの CHECK の両方を直す。
const (
	MaxCommentChars        = 2000
	MaxBurgerNameChars     = 100
	MaxShopNameChars       = 100
	MaxUsernameChars       = 50
	MaxEmailChars          = 254
	MaxModerationNoteChars = 500
)

// exceedsChars は s のコードポイント数が max を超えるかを返す。
func exceedsChars(s string, max int) bool {
	return utf8.RuneCountInString(s) > max
}

// tooLongMessage は上限超過の Rails 形式の full message を返す。メッセージは
// API の外部契約なので英語のままである（password の "is too long" と同じ形）。
func tooLongMessage(label string, max int) string {
	return fmt.Sprintf("%s is too long (maximum is %d characters)", label, max)
}
