package domain

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidOrigin は、Origin(ブラウザが要求に付ける、要求元の scheme・host・port)として使えない値を表す。
// null(ブラウザが、要求元を明かさないときに送る値)・ワイルドカード・path や query を持つ値・http(s) 以外を含む。
var ErrInvalidOrigin = errors.New("origin is invalid")

// NormalizeOrigin は、Origin を、完全一致で比べられる形("scheme://host[:port]"。scheme と host は小文字、
// その scheme の既定のポート(http は 80、https は 443)は省く)にそろえる。scheme・host・port のどれかが違えば
// 別の Origin であり、部分一致は決してしない。
//
// 次のものは、使えない値として ErrInvalidOrigin を返す: 空・"null"・http(s) 以外の scheme・host がない・
// 利用者情報(user@)・path(末尾の "/" を含む)・query・fragment・空のポート("host:")・範囲外や 0 から始まる
// ポート。ブラウザが送る Origin は、これらを含まない正規の形なので、含むものは、ブラウザ以外が作った値である。
func NormalizeOrigin(raw string) (string, error) {
	if raw == "" || raw == "null" || strings.ContainsAny(raw, "?#") {
		return "", ErrInvalidOrigin
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" ||
		u.Hostname() == "" || strings.HasSuffix(u.Host, ":") {
		return "", ErrInvalidOrigin
	}
	scheme := strings.ToLower(u.Scheme)
	defaultPort := ""
	switch scheme {
	case "http":
		defaultPort = "80"
	case "https":
		defaultPort = "443"
	default:
		return "", ErrInvalidOrigin
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return "", ErrInvalidOrigin
		}
	}
	if port == defaultPort {
		port = ""
	}
	if port == "" {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return scheme + "://" + host, nil
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}
