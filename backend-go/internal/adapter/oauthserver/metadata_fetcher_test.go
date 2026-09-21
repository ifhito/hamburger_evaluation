package oauthserver

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func allowAll(netip.Addr) bool { return true }

// testFetcher は、手元のテスト用の TLS サーバー srv に接続できる取得役を返す。allow で、
// 接続先の許可の判断を差し替えられる。
func testFetcher(srv *httptest.Server, allow func(netip.Addr) bool, timeout time.Duration) *HTTPMetadataFetcher {
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	cfg.MinVersion = tls.VersionTLS12
	return newHTTPMetadataFetcher(allow, cfg, timeout)
}

func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}

func TestHTTPMetadataFetcherGet(t *testing.T) {
	ctx := context.Background()

	t.Run("状態が 200 で、種類が JSON の応答は、本文をそのまま返す", func(t *testing.T) {
		srv := httptest.NewTLSServer(jsonHandler(`{"client_id":"x"}`))
		defer srv.Close()
		body, err := testFetcher(srv, allowAll, time.Second).get(ctx, srv.URL)
		if err != nil || string(body) != `{"client_id":"x"}` {
			t.Errorf("body = %q, err = %v", body, err)
		}
	})

	t.Run("本番の設定では、ループバックのサーバーには接続せず、リクエストが相手に届かない", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
		defer srv.Close()
		_, err := testFetcher(srv, domain.IsPublicAddress, time.Second).get(ctx, srv.URL)
		if !errors.Is(err, errBlockedAddress) {
			t.Errorf("err = %v, want errBlockedAddress", err)
		}
		if hits.Load() != 0 {
			t.Errorf("サーバーに %d 回届いた, want 0", hits.Load())
		}
	})

	t.Run("名前で指定しても、名前解決の結果がループバックなら、接続の直前に断る(名前の付け替えによる攻撃を防ぐ)", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
		defer srv.Close()
		port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
		_, err := testFetcher(srv, domain.IsPublicAddress, time.Second).get(ctx, "https://localhost:"+port+"/client.json")
		if !errors.Is(err, errBlockedAddress) || hits.Load() != 0 {
			t.Errorf("err = %v, hits = %d, want errBlockedAddress and 0 hits", err, hits.Load())
		}
	})

	t.Run("リダイレクトは追わず、エラーにする(リダイレクト先の内部サーバーには届かない)", func(t *testing.T) {
		var internalHits atomic.Int32
		mux := http.NewServeMux()
		mux.HandleFunc("/internal", func(w http.ResponseWriter, r *http.Request) { internalHits.Add(1) })
		mux.HandleFunc("/client.json", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/internal", http.StatusFound)
		})
		srv := httptest.NewTLSServer(mux)
		defer srv.Close()
		_, err := testFetcher(srv, allowAll, time.Second).get(ctx, srv.URL+"/client.json")
		if err == nil || !strings.Contains(err.Error(), "302") {
			t.Errorf("err = %v, want an unexpected status 302 error", err)
		}
		if internalHits.Load() != 0 {
			t.Errorf("リダイレクト先に %d 回届いた, want 0", internalHits.Load())
		}
	})

	t.Run("環境変数でプロキシが指定されていても、プロキシを経由せずに直接接続する", func(t *testing.T) {
		var proxyHits atomic.Int32
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyHits.Add(1) }))
		defer proxy.Close()
		t.Setenv("HTTPS_PROXY", proxy.URL)
		t.Setenv("https_proxy", proxy.URL)
		srv := httptest.NewTLSServer(jsonHandler(`{}`))
		defer srv.Close()
		if _, err := testFetcher(srv, allowAll, time.Second).get(ctx, srv.URL); err != nil {
			t.Fatal(err)
		}
		if proxyHits.Load() != 0 {
			t.Errorf("プロキシに %d 回届いた, want 0", proxyHits.Load())
		}
	})

	failures := map[string]http.HandlerFunc{
		"状態が 404 の応答は、エラーにする": func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"状態が 500 の応答は、エラーにする": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) },
		"種類が JSON ではない(HTML)応答は、エラーにする": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`{}`))
		},
		"種類の指定がない応答は、エラーにする": func(w http.ResponseWriter, r *http.Request) {
			w.Header()["Content-Type"] = nil
			_, _ = w.Write([]byte(`{}`))
		},
		"本文が上限を 1 バイトでも超える応答は、エラーにする": jsonHandler(strings.Repeat("a", domain.MaxClientMetadataBytes+1)),
	}
	for name, h := range failures {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(h)
			defer srv.Close()
			if body, err := testFetcher(srv, allowAll, time.Second).get(ctx, srv.URL); err == nil {
				t.Errorf("エラーにならなかった (body %d バイト)", len(body))
			}
		})
	}

	t.Run("本文がちょうど上限のバイト数なら、読み込める", func(t *testing.T) {
		srv := httptest.NewTLSServer(jsonHandler(strings.Repeat("a", domain.MaxClientMetadataBytes)))
		defer srv.Close()
		body, err := testFetcher(srv, allowAll, time.Second).get(ctx, srv.URL)
		if err != nil || len(body) != domain.MaxClientMetadataBytes {
			t.Errorf("len = %d, err = %v", len(body), err)
		}
	})

	t.Run("応答が待ち時間の上限を過ぎても終わらないときは、エラーにする", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		}))
		defer srv.Close()
		start := time.Now()
		_, err := testFetcher(srv, allowAll, 150*time.Millisecond).get(ctx, srv.URL)
		if err == nil {
			t.Fatal("エラーにならなかった")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("エラーになるまで %v かかった, want about 150ms", elapsed)
		}
	})
}

func TestHTTPMetadataFetcherFetchRejectsUnsafeURLs(t *testing.T) {
	f := NewHTTPMetadataFetcher()
	for name, u := range map[string]string{
		"http の URL は、接続する前に断る":             "http://app.example.com/client.json",
		"IP アドレスの URL は、接続する前に断る":           "https://93.184.216.34/client.json",
		"クラウドの情報取得用のアドレスは、接続する前に断る":         "https://169.254.169.254/latest/meta-data",
		"標準以外のポートの URL は、接続する前に断る":          "https://app.example.com:8443/client.json",
		"認証情報つきの URL は、接続する前に断る":            "https://user:pass@app.example.com/client.json",
		"経路がない URL は、接続する前に断る":              "https://app.example.com",
		"localhost への URL(名前がループバックを指す)は断る": "https://localhost/client.json",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.Fetch(context.Background(), u)
			if err == nil {
				t.Fatal("エラーにならなかった")
			}
			// 形で断れるものは ErrOAuthClientInvalid、名前解決の結果で断るものは接続の拒否になる。どちらも接続していない。
			if !errors.Is(err, domain.ErrOAuthClientInvalid) && !errors.Is(err, errBlockedAddress) {
				t.Errorf("err = %v, want ErrOAuthClientInvalid or errBlockedAddress", err)
			}
		})
	}
}
