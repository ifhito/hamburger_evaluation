// @vitest-environment node
//
// Worker(frontend/worker/index.js)の転送の約束を固める。デプロイ後の確認では
// GET と 302 の一部しか見られないので、ここで POST・クエリ・ヘッダー・応答の素通しまで見る。
// Cloudflare の実行環境は使わず、fetch と env.ASSETS を差し替えて呼ぶ。
import { afterEach, describe, expect, it, vi } from 'vitest'
const { getIDToken } = vi.hoisted(() => ({ getIDToken: vi.fn() }))
vi.mock('./google-identity.js', () => ({ createGoogleIdentityProvider: () => getIDToken }))
import worker from './index.js'

const API_ORIGIN = 'https://api-test.run.app'

function setup() {
  getIDToken.mockReset().mockResolvedValue('google-id-token')
  const upstream = vi.fn(async () => new Response('from-api'))
  vi.stubGlobal('fetch', upstream)
  const assets = { fetch: vi.fn(async () => new Response('from-assets')) }
  return { upstream, assets, env: { API_ORIGIN, ASSETS: assets } }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('worker の /api 転送', () => {
  it('/api を外して、クエリごと API へ渡す', async () => {
    const { upstream, env } = setup()
    await worker.fetch(new Request('https://front.test/api/reviews?page=2&per_page=20'), env)

    const [req] = upstream.mock.calls[0]
    expect(req.url).toBe(`${API_ORIGIN}/reviews?page=2&per_page=20`)
  })

  it('/api/ だけなら API のルートへ渡す', async () => {
    const { upstream, env } = setup()
    await worker.fetch(new Request('https://front.test/api/'), env)

    expect(upstream.mock.calls[0][0].url).toBe(`${API_ORIGIN}/`)
  })

  it('POST の本文とヘッダー(認証)をそのまま渡す', async () => {
    const { upstream, env } = setup()
    await worker.fetch(
      new Request('https://front.test/api/reviews', {
        method: 'POST',
        headers: { Authorization: 'Bearer t', 'Content-Type': 'application/json' },
        body: JSON.stringify({ rating: 5 }),
      }),
      env,
    )

    const [req] = upstream.mock.calls[0]
    expect(req.method).toBe('POST')
    expect(req.headers.get('Authorization')).toBe('Bearer t')
    expect(await req.text()).toBe('{"rating":5}')
  })

  it('リダイレクトを追いかけない(Google ログインの 302 をブラウザへ返すため)', async () => {
    const { upstream, env } = setup()
    await worker.fetch(new Request('https://front.test/api/auth/google/start'), env)

    expect(upstream.mock.calls[0][1]).toEqual({ redirect: 'manual' })
  })

  it('API の応答(302 の Location・Set-Cookie)を書き換えずに返す', async () => {
    const { env } = setup()
    const apiResponse = new Response(null, {
      status: 302,
      headers: { Location: 'https://accounts.google.com/o/oauth2/auth', 'Set-Cookie': 'state=x; Path=/api' },
    })
    vi.stubGlobal('fetch', vi.fn(async () => apiResponse))

    const res = await worker.fetch(new Request('https://front.test/api/auth/google/start'), env)

    expect(res).toBe(apiResponse)
  })

  it('/api 以外は静的ファイルの配信に任せる', async () => {
    const { upstream, assets, env } = setup()
    const req = new Request('https://front.test/reviews/123')
    const res = await worker.fetch(req, env)

    expect(upstream).not.toHaveBeenCalled()
    expect(assets.fetch).toHaveBeenCalledWith(req)
    expect(await res.text()).toBe('from-assets')
  })

  it('/apiary のような、/api/ で始まらないパスは転送しない', async () => {
    const { upstream, env } = setup()
    await worker.fetch(new Request('https://front.test/apiary'), env)

    expect(upstream).not.toHaveBeenCalled()
  })
})

 describe('IAM認証とMCPの発見用URL', () => {
  it('利用者の認証を保持し、外部からのインフラ認証ヘッダーは上書きする', async () => {
   const { upstream, env } = setup()
   await worker.fetch(new Request('https://front.test/api/mcp', { headers: {
    Authorization: 'Bearer user-token', 'X-Serverless-Authorization': 'Bearer attacker',
   } }), env)
   const [req] = upstream.mock.calls[0]
   expect(req.headers.get('Authorization')).toBe('Bearer user-token')
   expect(req.headers.get('X-Serverless-Authorization')).toBe('Bearer google-id-token')
   expect(getIDToken).toHaveBeenCalledWith(env, API_ORIGIN)
  })
  it('Google認証に失敗したときは匿名で転送せず503を返す', async () => {
   const { upstream, env } = setup()
   getIDToken.mockRejectedValue(new Error('private-key-material'))
   const res = await worker.fetch(new Request('https://front.test/api/meta'), env)
   expect(res.status).toBe(503)
   expect(await res.text()).not.toContain('private-key-material')
   expect(upstream).not.toHaveBeenCalled()
  })
  it.each([
   ['/.well-known/oauth-authorization-server/api', '/.well-known/oauth-authorization-server'],
   ['/.well-known/oauth-authorization-server', '/.well-known/oauth-authorization-server'],
   ['/.well-known/oauth-protected-resource/api/mcp', '/.well-known/oauth-protected-resource/api/mcp'],
   ['/.well-known/oauth-protected-resource', '/.well-known/oauth-protected-resource'],
  ])('%sをSPAではなくAPIへ渡す', async (path, expected) => {
   const { upstream, assets, env } = setup()
   await worker.fetch(new Request('https://front.test'+path), env)
   expect(upstream.mock.calls[0][0].url).toBe(API_ORIGIN+expected)
   expect(assets.fetch).not.toHaveBeenCalled()
  })
  it('二重スラッシュのパスでも認証情報を別ホストへ送らない', async () => {
   const { upstream, env } = setup()
   await worker.fetch(new Request('https://front.test/api//evil.test/path'), env)
   expect(new URL(upstream.mock.calls[0][0].url).origin).toBe(API_ORIGIN)
  })
  it.each(['http://api-test.run.app', 'https://evil.test', 'https://api-test.run.app/path',
   'https://user:pass@api-test.run.app'])('不正な転送先%sには認証情報を送らない', async origin => {
   const { upstream, env } = setup()
   env.API_ORIGIN = origin
   const res = await worker.fetch(new Request('https://front.test/api/meta'), env)
   expect(res.status).toBe(503)
   expect(getIDToken).not.toHaveBeenCalled()
   expect(upstream).not.toHaveBeenCalled()
  })
 })

const SAMPLE_HTML = `<!doctype html>
<html lang="ja">
  <head>
    <meta charset="UTF-8" />
    <title>BurgerStack</title>
    <meta name="description" content="既定の説明" />
    <meta property="og:title" content="BurgerStack" />
    <meta property="og:image" content="https://burger-stack.com/ogp/default.png" />
    <meta name="twitter:card" content="summary_large_image" />
  </head>
  <body><div id="root"></div></body>
</html>`

const REVIEW_UUID = '11111111-2222-3333-4444-555555555555'
const BURGER_UUID = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'
const SHOP_UUID = '99999999-8888-7777-6666-555555555555'

describe('公開詳細ページの動的 OGP タグ注入', () => {
  it('写真つきレビュー詳細にアクセスすると、投稿写真の og:image とレビュー情報のカードタグを埋め込む', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockResolvedValue(
      new Response(SAMPLE_HTML, { status: 200, headers: { 'Content-Type': 'text/html; charset=utf-8' } }),
    )
    const upstream = vi.fn(async () =>
      new Response(
        JSON.stringify({
          id: REVIEW_UUID,
          rating: 5,
          comment: '肉汁たっぷりで最高でした！',
          photo_url: 'https://r2.burger-stack.test/reviews/photo-1.webp',
          user: { id: 'u1', username: 'alice' },
          burger: { id: BURGER_UUID, name: 'ダブルチーズバーガー' },
          shop: { id: SHOP_UUID, name: 'Burger Mania 恵比寿' },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    vi.stubGlobal('fetch', upstream)

    const res = await worker.fetch(new Request(`https://front.test/reviews/${REVIEW_UUID}?utm_source=x`), env)
    const html = await res.text()

    expect(upstream).toHaveBeenCalledTimes(1)
    expect(upstream.mock.calls[0][0].url).toBe(`${API_ORIGIN}/reviews/${REVIEW_UUID}`)
    expect(html).toContain('<title>🍔 ダブルチーズバーガー（Burger Mania 恵比寿） ★5 | BurgerStack</title>')
    expect(html).toContain('<meta property="og:image" content="https://r2.burger-stack.test/reviews/photo-1.webp" />')
    expect(html).toContain('<meta name="twitter:image" content="https://r2.burger-stack.test/reviews/photo-1.webp" />')
    expect(html).toContain('<meta name="twitter:card" content="summary_large_image" />')
    expect(html).toContain(`<meta property="og:url" content="https://front.test/reviews/${REVIEW_UUID}" />`)
    expect(html).toContain('肉汁たっぷりで最高でした！（alice）')
    // 既定の og:image が残って二重にならないことを確かめる
    expect(html.match(/property="og:image"/g)).toHaveLength(1)
  })

  it('写真なしレビューや不正スキームの photo_url では、既定の OGP 画像へフォールバックする', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockResolvedValue(
      new Response(SAMPLE_HTML, { status: 200, headers: { 'Content-Type': 'text/html' } }),
    )
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            id: REVIEW_UUID,
            rating: 4,
            comment: null,
            photo_url: 'javascript:alert(1)',
            user: { id: 'u1', username: 'bob' },
            burger: { id: BURGER_UUID, name: 'クラシックバーガー' },
            shop: null,
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )

    const res = await worker.fetch(new Request(`https://front.test/reviews/${REVIEW_UUID}`), env)
    const html = await res.text()

    expect(html).toContain('<meta property="og:image" content="https://front.test/ogp/default.png" />')
    expect(html).toContain('bobさんのハンバーガーレビュー | BurgerStack')
  })

  it('バーガー名やコメントに HTML 特殊文字が含まれていてもエスケープして XSS を防ぐ', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockResolvedValue(
      new Response(SAMPLE_HTML, { status: 200, headers: { 'Content-Type': 'text/html' } }),
    )
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            id: REVIEW_UUID,
            rating: 3,
            comment: '"><script>alert("xss")</script>',
            photo_url: null,
            user: { id: 'u1', username: 'eve' },
            burger: { id: BURGER_UUID, name: '</title><img src=x onerror=alert(1)>' },
            shop: null,
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )

    const res = await worker.fetch(new Request(`https://front.test/reviews/${REVIEW_UUID}`), env)
    const html = await res.text()

    expect(html).not.toContain('<script>alert("xss")</script>')
    expect(html).not.toContain('<img src=x onerror=alert(1)>')
    expect(html).toContain('&lt;/title&gt;&lt;img src=x onerror=alert(1)&gt;')
    expect(html).toContain('&quot;&gt;&lt;script&gt;alert(&quot;xss&quot;)&lt;/script&gt;')
  })

  it('OGP 取得時は閲覧者の Authorization や Cookie を API に転送せず公開情報のみを取得する', async () => {
    const { upstream, env } = setup()
    env.ASSETS.fetch.mockResolvedValue(
      new Response(SAMPLE_HTML, { status: 200, headers: { 'Content-Type': 'text/html' } }),
    )
    upstream.mockResolvedValue(
      new Response(JSON.stringify({ id: SHOP_UUID, name: 'Public Shop', review_count: 2, average_rating: 4.5, photo_url: null }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const res = await worker.fetch(
      new Request(`https://front.test/shops/${SHOP_UUID}`, {
        headers: { Authorization: 'Bearer secret-jwt', Cookie: 'session=secret' },
      }),
      env,
    )
    const [apiReq] = upstream.mock.calls[0]
    expect(apiReq.headers.get('Authorization')).toBeNull()
    expect(apiReq.headers.get('Cookie')).toBeNull()
    expect(apiReq.headers.get('X-Serverless-Authorization')).toBe('Bearer google-id-token')
    expect(await res.text()).toContain('🍔 Public Shop | BurgerStack')
  })

  it('バーガー詳細ページでも平均評価・件数・写真の OGP タグを埋め込む', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockResolvedValue(
      new Response(SAMPLE_HTML, { status: 200, headers: { 'Content-Type': 'text/html' } }),
    )
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            id: BURGER_UUID,
            name: 'アボカドバーガー',
            shops: [{ id: SHOP_UUID, name: 'Burger Mania 恵比寿' }],
            photo_url: 'https://r2.burger-stack.test/reviews/avocado.webp',
            average_rating: 4.3,
            review_count: 8,
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )

    const res = await worker.fetch(new Request(`https://front.test/burgers/${BURGER_UUID}`), env)
    const html = await res.text()

    expect(html).toContain('<title>🍔 アボカドバーガー（Burger Mania 恵比寿） | BurgerStack</title>')
    expect(html).toContain('平均評価 ★4.3（8件のレビュー）')
    expect(html).toContain('<meta property="og:image" content="https://r2.burger-stack.test/reviews/avocado.webp" />')
  })

  it('審査待ち店舗など API が 404 を返すときや API 障害時は、エラー画面にせず元の SPA HTML をそのまま返す', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockImplementation(
      async () =>
        new Response(SAMPLE_HTML, {
          status: 200,
          headers: { 'Content-Type': 'text/html', ETag: '"static-etag"' },
        }),
    )
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"not found"}', { status: 404 })))

    const res404 = await worker.fetch(new Request(`https://front.test/shops/${SHOP_UUID}`), env)
    expect(res404.status).toBe(200)
    expect(res404.headers.get('ETag')).toBeNull()
    expect(res404.headers.get('Cache-Control')).toBe('public, max-age=0, must-revalidate')
    expect(await res404.text()).toBe(SAMPLE_HTML)

    // 通信例外や IAM 失敗時でも 503/500 にせず SPA HTML を返す
    vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('network timeout') }))
    const resErr = await worker.fetch(new Request(`https://front.test/reviews/${REVIEW_UUID}`), env)
    expect(resErr.status).toBe(200)
    expect(await resErr.text()).toBe(SAMPLE_HTML)
  })

  it('If-None-Match 付きの詳細ページ要求でも 304 クラッシュを起こさず最新の動的 OGP HTML を返す', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockImplementation(async (req) => {
      // ASSETS 側へ条件ヘッダーが渡っていないことと、ルート '/' を取得していることを確かめる
      expect(new URL(req.url).pathname).toBe('/')
      expect(req.headers.get('If-None-Match')).toBeNull()
      return new Response(SAMPLE_HTML, {
        status: 200,
        headers: { 'Content-Type': 'text/html', ETag: '"static-etag"' },
      })
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            id: SHOP_UUID,
            name: 'Approved Shop',
            review_count: 1,
            average_rating: 5.0,
            photo_url: '/photos/reviews/local.jpg',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )

    const res = await worker.fetch(
      new Request(`https://front.test/shops/${SHOP_UUID}`, {
        headers: { 'If-None-Match': '"static-etag"' },
      }),
      env,
    )
    expect(res.status).toBe(200)
    expect(res.headers.get('ETag')).toBeNull()
    const html = await res.text()
    expect(html).toContain('🍔 Approved Shop | BurgerStack')
    expect(html).toContain('<meta property="og:image" content="https://front.test/photos/reviews/local.jpg" />')
  })

  it('バックスラッシュを含む不正な相対 photo_url(/\\evil.test/...)は別オリジンへ解決せず既定画像へフォールバックする', async () => {
    const { env } = setup()
    env.ASSETS.fetch.mockResolvedValue(
      new Response(SAMPLE_HTML, { status: 200, headers: { 'Content-Type': 'text/html' } }),
    )
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            id: SHOP_UUID,
            name: 'Test Shop',
            review_count: 0,
            average_rating: null,
            photo_url: '/\\evil.test/steal.png',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )

    const res = await worker.fetch(new Request(`https://front.test/shops/${SHOP_UUID}`), env)
    const html = await res.text()
    expect(html).toContain('<meta property="og:image" content="https://front.test/ogp/default.png" />')
    expect(html).not.toContain('evil.test')
  })

  it('/reviews/new や /reviews/:id/edit のような非詳細パスでは API を呼ばず静的配信に任せる', async () => {
    const { upstream, assets, env } = setup()
    await worker.fetch(new Request('https://front.test/reviews/new?shop_id=' + SHOP_UUID), env)
    await worker.fetch(new Request(`https://front.test/reviews/${REVIEW_UUID}/edit`), env)

    expect(upstream).not.toHaveBeenCalled()
    expect(assets.fetch).toHaveBeenCalledTimes(2)
  })
})
