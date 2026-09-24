package domain

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func errMessages(err error) []string {
	if err == nil {
		return nil
	}
	return err.(*ValidationError).Texts(LangEN)
}

// limitCase は、1 つの検証関数の上限の境界を確かめる入力である。
type limitCase struct {
	name     string
	label    string // 上限超過のメッセージの先頭（"Comment" など）
	max      int
	validate func(s string) []string
	// build は、unit を使って、全体が n 文字の値を作る。
	build func(unit string, n int) string
	units []string
}

func repeatUnit(unit string, n int) string { return strings.Repeat(unit, n) }

const emailDomain = "@example.com"

var (
	allUnits   = []string{"a", "あ", "🍔"} // 1・3・4 バイト
	asciiUnits = []string{"a"}
)

var limitCases = []limitCase{
	{"コメント", "Comment", MaxCommentChars, func(s string) []string { return errMessages(ValidateReviewContent(3, s)) }, repeatUnit, allUnits},
	{"バーガー名", "Burger name", MaxBurgerNameChars, func(s string) []string { return errMessages(ValidateBurgerName(s)) }, repeatUnit, allUnits},
	{"ショップ名", "Name", MaxShopNameChars, func(s string) []string { return errMessages(ValidateShopName(s)) }, repeatUnit, allUnits},
	{"ユーザー名", "Username", MaxUsernameChars, ValidateUsername, repeatUnit, allUnits},
	{"自己紹介文", "Bio", MaxBioChars, ValidateBio, repeatUnit, allUnits},
	{"却下メモ", "Moderation note", MaxModerationNoteChars, func(s string) []string { return errMessages(ValidateModerationNote(&s)) }, repeatUnit, allUnits},
	// email は形式としても有効でなければならないので、ASCII のローカル部を伸ばして全体の文字数を合わせる。
	{"メール", "Email", MaxEmailChars, ValidateEmail, func(unit string, n int) string {
		return strings.Repeat(unit, n-len(emailDomain)) + emailDomain
	}, asciiUnits},
}

// TestTextLimits は、各項目で「上限ちょうどは有効」「1 文字超えると上限のメッセージだけ」を固定する。
// 文字数はコードポイント数で数えるので、日本語（3 バイト）や絵文字（4 バイト）でも、
// 文字数が上限ちょうどなら有効で、バイト数では上限を超える値も通る。
func TestTextLimits(t *testing.T) {
	for _, tc := range limitCases {
		for _, unit := range tc.units {
			t.Run(tc.name+"は上限ちょうどの"+strconv.Quote(unit)+"なら有効", func(t *testing.T) {
				if got := tc.validate(tc.build(unit, tc.max)); got != nil {
					t.Errorf("上限ちょうど（%d 文字）が無効: %v", tc.max, got)
				}
			})
			t.Run(tc.name+"は上限を 1 文字超えた"+strconv.Quote(unit)+"なら上限のメッセージだけを返す", func(t *testing.T) {
				want := []string{tc.label + " is too long (maximum is " + strconv.Itoa(tc.max) + " characters)"}
				if got := tc.validate(tc.build(unit, tc.max+1)); !reflect.DeepEqual(got, want) {
					t.Errorf("got %v, want %v", got, want)
				}
			})
		}
	}
}

// TestTextLimitsCountCodePoints は、数え方がコードポイント数であることを固定する:
// 結合文字は 1 文字として数え（書記素クラスタにはまとめない）、絵文字は通常 1 文字である。
func TestTextLimitsCountCodePoints(t *testing.T) {
	// "e" + 結合アクセント(U+0301) は 2 コードポイント（見た目は 1 文字）。
	combining := "é"
	if !exceedsChars(strings.Repeat(combining, MaxUsernameChars/2)+"x", MaxUsernameChars) {
		t.Error("結合文字は 2 コードポイントとして数える(書記素クラスタにまとめない)")
	}
	if exceedsChars(strings.Repeat(combining, MaxUsernameChars/2), MaxUsernameChars) {
		t.Errorf("ちょうど %d コードポイントは上限内", MaxUsernameChars)
	}
}

// TestTextLimitsKeepExistingRules は、上限の追加が、空・複数の違反の並びを変えないこと(コメントは
// 空・空白のみでも有効であること)を固定する。
func TestTextLimitsKeepExistingRules(t *testing.T) {
	tooLongComment := strings.Repeat("a", MaxCommentChars+1)
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"rating の違反はコメントの上限超過より先に並ぶ", errMessages(ValidateReviewContent(6, tooLongComment)),
			[]string{"Rating must be in 1..5", "Comment is too long (maximum is 2000 characters)"}},
		{"空白のみのコメントは有効(空欄のレビューを許す)", errMessages(ValidateReviewContent(3, " ")), nil},
		{"空のユーザー名は blank だけ", ValidateUsername(""), []string{"Username can't be blank"}},
		{"空の email は blank だけ", ValidateEmail(""), []string{"Email can't be blank"}},
		{"note なしの reject は有効", errMessages(ValidateModerationNote(nil)), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}
