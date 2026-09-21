package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestWriteOAuthErrorFollowsTheLanguage は、許可の画面の API が返す、OAuth のエラーの応答を、エラーの種類ごとに、
// 固定した文字列で確かめる。英語は、これまでと同じ(エラーの文字列がそのまま)で、日本語は、日本語の説明に、その
// 診断の文を「詳細」として添える。範囲の不正は、要求の不正が先に拒まれるので、結合テストからは届かない経路である。
func TestWriteOAuthErrorFollowsTheLanguage(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		en, ja string
	}{
		{
			name: "アプリへ結果を戻せない要求は、診断の文つきで、422 になる",
			err:  fmt.Errorf("%w: the query string is malformed", domain.ErrOAuthAuthorizeRequestInvalid), status: http.StatusUnprocessableEntity,
			en: `{"error":"oauth authorization request is invalid: the query string is malformed"}`,
			ja: `{"error":"このアプリからの許可の要求が正しくありません。詳細: oauth authorization request is invalid: the query string is malformed"}`,
		},
		{
			name: "範囲の不正は、処理の名前が前に付いても、診断の文つきで、422 になる",
			err:  fmt.Errorf("decide oauth consent: %w", fmt.Errorf("%w: unknown scope %q", domain.ErrOAuthInvalidScope, "x")), status: http.StatusUnprocessableEntity,
			en: `{"error":"decide oauth consent: oauth scope is invalid: unknown scope \"x\""}`,
			ja: `{"error":"要求された許可の範囲が正しくありません。詳細: decide oauth consent: oauth scope is invalid: unknown scope \"x\""}`,
		},
		{
			name: "存在しない許可は、404 になる",
			err:  domain.ErrOAuthGrantNotFound, status: http.StatusNotFound,
			en: `{"error":"not found"}`, ja: `{"error":"見つかりません"}`,
		},
		{
			name: "そのほかのエラーは、言語によらず、英語の固定の 500 になる",
			err:  errors.New("boom"), status: http.StatusInternalServerError,
			en: `{"error":"internal server error"}`, ja: `{"error":"internal server error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for header, want := range map[string]string{"": tt.en, "ja": tt.ja, "fr-FR": tt.en} {
				req := httptest.NewRequest(http.MethodPost, "/oauth/authorize/decision", nil)
				if header != "" {
					req.Header.Set("Accept-Language", header)
				}
				rec := httptest.NewRecorder()
				writeOAuthError(rec, req, "decide", tt.err)
				if rec.Code != tt.status || rec.Body.String() != want {
					t.Errorf("Accept-Language %q: %d %s, want %d %s", header, rec.Code, rec.Body, tt.status, want)
				}
			}
		})
	}
}
