package domain

import (
	"net/url"
	"strings"
)

// returnToMaxBytes は、サインインのあとに戻る先(アプリの中のパス)の、バイト数の上限である。
const returnToMaxBytes = 2048

// SanitizeReturnTo は、サインインのあとに戻る先として渡された値が、このアプリの中のパスなら、
// そのまま返し、そうでなければ空文字列を返す(空は「既定の画面へ」の意味)。外部のサイトへ
// 飛ばされる(オープンリダイレクト)のを防ぐ規則で、判定は domain だけが持つ。
//
// 許すのは、"/" で始まる 1 つのパスだけである。次のものは、外部への移動になりうるので、すべて断る:
// "//host" と "/\host"(ブラウザは "\" を "/" として扱う)、"\" を含むもの、制御文字(ブラウザは、
// タブや改行を取り除いてから解釈する)、スキームやホストを持つもの("https://..."・"javascript:...")、
// 長すぎるもの。クエリは許す("/oauth/authorize?...")。
func SanitizeReturnTo(raw string) string {
	if raw == "" || len(raw) > returnToMaxBytes || raw[0] != '/' {
		return ""
	}
	if strings.HasPrefix(raw, "//") || strings.Contains(raw, `\`) {
		return ""
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	return raw
}
