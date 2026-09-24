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
