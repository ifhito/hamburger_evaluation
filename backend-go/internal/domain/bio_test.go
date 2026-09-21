package domain

import "testing"

// TestValidateBioFreeText は、自己紹介文が「利用者が自由に書く文章」として扱われることを固定する。
// 空文字（未設定にする・書いた内容を消す操作）、改行、HTML のタグに見える文字列、前後の空白は、
// いずれも拒否も加工もせず、そのまま有効とする。HTML はここでエスケープしない
// （表示する側が、HTML として解釈せず文字として描画する）。
// 文字数の上限は、limits_test.go の表が確かめる。
func TestValidateBioFreeText(t *testing.T) {
	for _, bio := range []string{
		"",
		"はじめまして。\n2 行目です。",
		"<script>alert(1)</script>",
		"  前後の空白は加工しない  ",
	} {
		if got := ValidateBio(bio); got != nil {
			t.Errorf("ValidateBio(%q) = %v, want nil", bio, got)
		}
	}
}
