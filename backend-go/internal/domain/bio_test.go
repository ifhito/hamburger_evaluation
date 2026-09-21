package domain

import "testing"

// TestValidateBioFreeText は、bio が「自由な文章」として扱われることを固定する。
// 空文字(未設定・消す操作)、改行、HTML に見える文字列は、そのまま有効である。
// 上限(MaxBioChars。日本語・絵文字はコードポイント数)は、limits_test.go の表が固定する。
// HTML は解釈も加工もしない(表示側が文字として描画する)。
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
