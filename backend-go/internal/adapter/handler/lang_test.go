package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func TestLangOf(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   domain.Lang
	}{
		{"指定がなければ英語(いまと同じ)", "", domain.LangEN},
		{"ja は日本語", "ja", domain.LangJA},
		{"地域つきの ja-JP も日本語", "ja-JP", domain.LangJA},
		{"大文字小文字は区別しない", "JA-jp", domain.LangJA},
		{"en は英語", "en", domain.LangEN},
		{"en-US も英語", "en-US", domain.LangEN},
		{"対応しない言語だけなら英語", "fr-FR,de;q=0.8", domain.LangEN},
		{"対応しない言語の次に日本語があれば日本語", "fr, ja", domain.LangJA},
		{"優先度(q)の高い方: 日本語", "en;q=0.5, ja;q=0.9", domain.LangJA},
		{"優先度(q)の高い方: 英語", "ja;q=0.4, en;q=0.9", domain.LangEN},
		{"同じ優先度なら書かれた順(日本語が先)", "ja, en", domain.LangJA},
		{"同じ優先度なら書かれた順(英語が先)", "en, ja", domain.LangEN},
		{"q=0 の言語は選ばない", "ja;q=0, en;q=0.1", domain.LangEN},
		{"q=0 の日本語だけなら英語", "ja;q=0", domain.LangEN},
		{"* は指定のないすべての言語なので、既定の英語として数える", "ja;q=0.5, *;q=0.9", domain.LangEN},
		{"読めない q の指定は無視する", "ja;q=abc", domain.LangEN},
		{"範囲外の q の指定は無視する", "ja;q=2, en;q=0.5", domain.LangEN},
		{"前後の空白があっても読める", "  ja-JP ; q=0.8 , en;q=0.7 ", domain.LangJA},
		{"空の指定が混ざっていても読める", ",,ja,", domain.LangJA},
		{"指定が多すぎるときは、先頭の一部だけを見る", strings.Repeat("fr,", maxLanguageRanges) + "ja", domain.LangEN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Accept-Language", tt.header)
			}
			if got := langOf(r); got != tt.want {
				t.Errorf("langOf(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}
