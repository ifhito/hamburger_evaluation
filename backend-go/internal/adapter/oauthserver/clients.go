package oauthserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ory/fosite"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// maxCachedClients は、取得したアプリの説明を覚えておく個数の上限である。アプリを名乗る URL は
// 誰でも作れるので、覚える個数に上限を置く。
const maxCachedClients = 256

// clientResolver は、client_id からアプリの登録情報を求める。固定で登録したアプリを先に探し、なければ、
// client_id が https の URL のとき、そのアプリが公開している説明の文書(CIMD)を取得して読む。
// 取得した内容は domain.ClientMetadataCacheTTL の間だけ覚える。
type clientResolver struct {
	static   map[string]domain.OAuthClient
	fetcher  MetadataFetcher
	resource string
	now      func() time.Time

	mu    sync.Mutex
	cache map[string]cachedClient
}

type cachedClient struct {
	client  domain.OAuthClient
	expires time.Time
}

func newClientResolver(static []domain.OAuthClient, fetcher MetadataFetcher, resource string, now func() time.Time) (*clientResolver, error) {
	m := make(map[string]domain.OAuthClient, len(static))
	for _, c := range static {
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("static oauth client %q: %w", c.ID, err)
		}
		m[c.ID] = c
	}
	return &clientResolver{static: m, fetcher: fetcher, resource: resource, now: now, cache: map[string]cachedClient{}}, nil
}

// resolve は、id のアプリの登録情報を返す。見つからない・文書が規則を満たさない・取得できないときはエラーを返す。
func (r *clientResolver) resolve(ctx context.Context, id string) (domain.OAuthClient, error) {
	if c, ok := r.static[id]; ok {
		return c, nil
	}
	if !strings.HasPrefix(id, "https://") {
		return domain.OAuthClient{}, fmt.Errorf("%w: unknown client", domain.ErrOAuthClientInvalid)
	}
	if c, ok := r.cached(id); ok {
		return c, nil
	}
	body, err := r.fetcher.Fetch(ctx, id)
	if err != nil {
		return domain.OAuthClient{}, err
	}
	c, err := domain.ParseClientMetadataDocument(id, body)
	if err != nil {
		return domain.OAuthClient{}, err
	}
	r.remember(c)
	return c, nil
}

func (r *clientResolver) cached(id string) (domain.OAuthClient, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[id]
	if !ok || !r.now().Before(e.expires) {
		delete(r.cache, id)
		return domain.OAuthClient{}, false
	}
	return e.client, true
}

func (r *clientResolver) remember(c domain.OAuthClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if len(r.cache) >= maxCachedClients {
		for id, e := range r.cache {
			if !now.Before(e.expires) {
				delete(r.cache, id)
			}
		}
	}
	if len(r.cache) >= maxCachedClients {
		return
	}
	r.cache[c.ID] = cachedClient{client: c, expires: now.Add(domain.ClientMetadataCacheTTL)}
}

// GetClient は、認可ライブラリが、client_id からアプリを求めるための口である。アプリが自分の説明の文書に
// 何を書いても、許す使い方・範囲・宛先は広がらない(newFositeClient)。
func (s *storage) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	c, err := s.clients.resolve(ctx, id)
	if err != nil {
		if !errors.Is(err, domain.ErrOAuthClientInvalid) {
			slog.Warn("oauth: client lookup failed", "client_id", truncate(id, 200), "error", err)
		}
		return nil, err
	}
	return newFositeClient(c.ID, c.RedirectURIs, s.clients.resource), nil
}

// newFositeClient は、アプリを、認可ライブラリの形にする。すべてのアプリを公開クライアント(秘密の鍵を
// 持たない)として扱い、許す使い方(認可コードと更新)・範囲・宛先を、domain の規則で決めた値に固定する。
// 保存した記録から認可を復元するときも、同じ値を使う(更新トークンを発行してよいかの判断に使われる)。
func newFositeClient(id string, redirectURIs []string, resource string) *fosite.DefaultClient {
	scopes := make([]string, 0, len(domain.OAuthScopes()))
	for _, sc := range domain.OAuthScopes() {
		scopes = append(scopes, sc.Name)
	}
	return &fosite.DefaultClient{
		ID:            id,
		RedirectURIs:  redirectURIs,
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		Scopes:        scopes,
		Audience:      []string{resource},
		Public:        true,
	}
}

// ClientAssertionJWTValid と SetClientAssertionJWT は、署名つきの文書でクライアントを認証する方式のための
// 口である。公開クライアントだけを扱い、この方式を有効にしていないので、何もしない。
func (s *storage) ClientAssertionJWTValid(context.Context, string) error { return nil }

func (s *storage) SetClientAssertionJWT(context.Context, string, time.Time) error { return nil }

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
