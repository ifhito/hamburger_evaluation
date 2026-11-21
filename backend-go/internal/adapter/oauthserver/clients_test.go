package oauthserver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubMetadataFetcher struct {
	fn func(ctx context.Context, clientIDURL string) ([]byte, error)
}

func (f stubMetadataFetcher) Fetch(ctx context.Context, clientIDURL string) ([]byte, error) {
	return f.fn(ctx, clientIDURL)
}

func validClientMetadataJSON(clientID string) []byte {
	return []byte(fmt.Sprintf(`{"client_id":%q,"client_name":"Test App","redirect_uris":["http://127.0.0.1/callback"]}`, clientID))
}

func TestClientResolverSingleflightAndCallerCancellation(t *testing.T) {
	const clientID = "https://client.example/oauth/metadata.json"
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var fetches atomic.Int32

	fetcher := stubMetadataFetcher{
		fn: func(ctx context.Context, url string) ([]byte, error) {
			fetches.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
				return validClientMetadataJSON(url), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	resolver, err := newClientResolver(nil, fetcher, "https://api.example/mcp", func() time.Time { return now })
	if err != nil {
		t.Fatalf("newClientResolver: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelErr := make(chan error, 1)
	go func() {
		_, err := resolver.resolve(cancelCtx, clientID)
		cancelErr <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("初回のメタデータ取得が始まらなかった")
	}

	var wg sync.WaitGroup
	waiterErrs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := resolver.resolve(context.Background(), clientID)
			if err != nil {
				waiterErrs <- err
				return
			}
			if c.ID != clientID {
				waiterErrs <- fmt.Errorf("resolved id = %q, want %q", c.ID, clientID)
			}
		}()
	}

	// 先行した呼び出し元だけをキャンセルしても、相乗りしている他の要求は巻き添えにならない。
	time.Sleep(15 * time.Millisecond)
	cancel()
	if err := <-cancelErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("キャンセルした呼び出し元のエラー = %v, want %v", err, context.Canceled)
	}

	close(release)
	wg.Wait()
	close(waiterErrs)
	for err := range waiterErrs {
		t.Errorf("相乗りしていた呼び出し元のエラー: %v", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("Fetch の呼び出し回数 = %d, want 1 (singleflight で統合される)", got)
	}
}

func TestClientResolverEvictsEarliestExpiringEntryWhenFull(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	fetcher := stubMetadataFetcher{
		fn: func(_ context.Context, url string) ([]byte, error) {
			mu.Lock()
			calls[url]++
			mu.Unlock()
			return validClientMetadataJSON(url), nil
		},
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	resolver, err := newClientResolver(nil, fetcher, "https://api.example/mcp", func() time.Time { return now })
	if err != nil {
		t.Fatalf("newClientResolver: %v", err)
	}

	urlFor := func(i int) string {
		return fmt.Sprintf("https://app-%03d.example/client.json", i)
	}

	// 有効期限内の項目で上限(maxCachedClients)まで埋める。0 番が最も早く期限切れになる。
	for i := 0; i < maxCachedClients; i++ {
		now = now.Add(time.Millisecond)
		if _, err := resolver.resolve(context.Background(), urlFor(i)); err != nil {
			t.Fatalf("resolve %d: %v", i, err)
		}
	}

	// 上限を超えた新規の 1 件もキャッシュに保存され、2 回目の解決では再取得しない。
	now = now.Add(time.Millisecond)
	overflowURL := urlFor(maxCachedClients)
	for i := 0; i < 2; i++ {
		if _, err := resolver.resolve(context.Background(), overflowURL); err != nil {
			t.Fatalf("resolve overflow (%d): %v", i, err)
		}
	}
	if got := calls[overflowURL]; got != 1 {
		t.Errorf("上限到達後に追加した URL の Fetch 回数 = %d, want 1 (キャッシュが無力化されていない)", got)
	}

	// 比較的新しい項目(1 番)はキャッシュに残り、最も古かった 0 番だけが追い出されている。
	if _, err := resolver.resolve(context.Background(), urlFor(1)); err != nil {
		t.Fatalf("resolve 1: %v", err)
	}
	if got := calls[urlFor(1)]; got != 1 {
		t.Errorf("残っているはずの 1 番の Fetch 回数 = %d, want 1", got)
	}
	if _, err := resolver.resolve(context.Background(), urlFor(0)); err != nil {
		t.Fatalf("resolve 0 again: %v", err)
	}
	if got := calls[urlFor(0)]; got != 2 {
		t.Errorf("最も早く期限切れになる 0 番の Fetch 回数 = %d, want 2 (追い出されて再取得される)", got)
	}
}
