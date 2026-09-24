package domain

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/msgcheck"
)

// TestCatalogStructure は、カタログの構造(両言語・値の並び・キーの定数・コードの使い方)を確かめる。
// 検査の中身は internal/testutil/msgcheck(handler のカタログと同じ実装)にある。
func TestCatalogStructure(t *testing.T) {
	entries := map[string]msgcheck.Entry{}
	for key, e := range catalog {
		entries[key] = msgcheck.Entry{EN: e.EN, JA: e.JA}
	}
	msgcheck.Run(t, msgcheck.Config{
		Catalog:  entries,
		KeyFile:  "messages.go",
		Dirs:     []string{".", filepath.Join("..", "usecase")},
		DirectOK: []string{"message.go"},
	})
}

func TestMessageTextFollowsTheLanguage(t *testing.T) {
	m := Msg(keyCommentTooLong, 2000)
	if got, want := m.Text(LangEN), "Comment is too long (maximum is 2000 characters)"; got != want {
		t.Errorf("英語 = %q, want %q", got, want)
	}
	if got, want := m.Text(LangJA), "コメントが長すぎます(最大 2000 文字)"; got != want {
		t.Errorf("日本語 = %q, want %q", got, want)
	}
	if got, want := m.Text(Lang("fr")), m.Text(LangEN); got != want {
		t.Errorf("未対応の言語 = %q, want 英語 %q", got, want)
	}
	if got := Msg("no.such.key").Text(LangJA); got != "no.such.key" {
		t.Errorf("カタログにないキー = %q, want キーそのもの", got)
	}
}

func TestValidationErrorTextsFollowTheLanguage(t *testing.T) {
	err := NewValidationError(Msg(keyReviewRatingRange, MinRating, MaxRating), Msg(keyCommentTooLong, 2000))
	if want := []string{"Rating must be in 1..5", "Comment is too long (maximum is 2000 characters)"}; !reflect.DeepEqual(err.Texts(LangEN), want) {
		t.Errorf("Texts(en) = %v, want %v", err.Texts(LangEN), want)
	}
	if want := []string{"評価は 1〜5 の整数で指定してください", "コメントが長すぎます(最大 2000 文字)"}; !reflect.DeepEqual(err.Texts(LangJA), want) {
		t.Errorf("Texts(ja) = %v, want %v", err.Texts(LangJA), want)
	}
	if want := "validation failed: Rating must be in 1..5, Comment is too long (maximum is 2000 characters)"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestValidatorsReturnJapaneseIssues(t *testing.T) {
	tests := []struct {
		name string
		got  []Message
		want []string
	}{
		{"パスワードが空", PasswordIssues(""), []string{"パスワードを入力してください"}},
		{"パスワードが短く、文字種も足りない", PasswordIssues("abc"), []string{"パスワードが短すぎます(最小 8 バイト)", "パスワードには、半角の英字・数字・記号を、それぞれ 1 文字以上含めてください"}},
		{"メールが空", EmailIssues(""), []string{"メールアドレスを入力してください"}},
		{"メールの形式が不正", EmailIssues("abc"), []string{"メールアドレスの形式が正しくありません"}},
		{"ユーザー名が空", UsernameIssues(""), []string{"ユーザー名を入力してください"}},
		{"自己紹介が長すぎる", BioIssues(strings.Repeat("あ", MaxBioChars+1)), []string{"自己紹介が長すぎます(最大 500 文字)"}},
		{"認証情報は、メール → パスワードの順に並ぶ", CredentialsIssues("", ""), []string{"メールアドレスを入力してください", "パスワードを入力してください"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Texts(LangJA, tt.got); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("日本語 = %v, want %v", got, tt.want)
			}
		})
	}
}
