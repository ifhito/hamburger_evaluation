package oauthserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// errBlockedAddress は、接続先が公開のインターネットのアドレスではないことを表す。
var errBlockedAddress = errors.New("connection to a non-public address is not allowed")

// MetadataFetcher は、アプリが公開している説明の文書(CIMD)を取得する口である。
type MetadataFetcher interface {
	// Fetch は、client_id の URL から文書の本文を取得する。
	Fetch(ctx context.Context, clientIDURL string) ([]byte, error)
}

// HTTPMetadataFetcher は、利用者(アプリ)が指定した URL を、サーバーが取りに行く実装である。
// 内部のサーバーへ向けさせる攻撃(SSRF)を防ぐため、次を必ず守る:
//   - URL の形の確認(https・標準のポート・ホスト名・パスあり。domain.ValidateClientMetadataURL)。
//   - 接続のたびに、名前解決の結果のアドレスが公開のインターネットのものかを確認する
//     (domain.IsPublicAddress)。名前解決の後・接続の直前に確認するので、確認のあとに別のアドレスへ
//     切り替える攻撃(DNS rebinding)も防げる。
//   - リダイレクトは追わない(3xx はエラー)。
//   - プロキシは使わない。
//   - 待ち時間と本文の大きさに上限を設ける(domain.ClientMetadataFetchTimeout・MaxClientMetadataBytes)。
type HTTPMetadataFetcher struct {
	client *http.Client
}

// NewHTTPMetadataFetcher は、本番の設定の取得役を返す。
func NewHTTPMetadataFetcher() *HTTPMetadataFetcher {
	return newHTTPMetadataFetcher(domain.IsPublicAddress, nil, domain.ClientMetadataFetchTimeout)
}

// newHTTPMetadataFetcher は、接続先の許可の判断 allow・TLS の設定 tlsConfig・待ち時間 timeout を
// 差し替えられる。テストが、手元のテスト用サーバーへ接続するために使う。
func newHTTPMetadataFetcher(allow func(netip.Addr) bool, tlsConfig *tls.Config, timeout time.Duration) *HTTPMetadataFetcher {
	dialer := &net.Dialer{
		Timeout: timeout,
		// Control は、名前解決のあと、実際に接続する直前に、接続先のアドレスごとに呼ばれる。
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errBlockedAddress
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !allow(ip) {
				return errBlockedAddress
			}
			return nil
		},
	}
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &HTTPMetadataFetcher{client: &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:             nil,
			DialContext:       dialer.DialContext,
			TLSClientConfig:   tlsConfig,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Fetch は、URL の形を確かめてから、文書を取得する。
func (f *HTTPMetadataFetcher) Fetch(ctx context.Context, clientIDURL string) ([]byte, error) {
	if err := domain.ValidateClientMetadataURL(clientIDURL); err != nil {
		return nil, err
	}
	return f.get(ctx, clientIDURL)
}

// get は、形の確認を済ませた URL から文書を取得する。ステータスが 200 で、種類が application/json で、
// 本文が上限以内のときだけ本文を返す。
func (f *HTTPMetadataFetcher) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch client metadata: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch client metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch client metadata: unexpected status %d", resp.StatusCode)
	}
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return nil, errors.New("fetch client metadata: content type must be application/json")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.MaxClientMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch client metadata: read body: %w", err)
	}
	if len(body) > domain.MaxClientMetadataBytes {
		return nil, errors.New("fetch client metadata: body is too large")
	}
	return body, nil
}
