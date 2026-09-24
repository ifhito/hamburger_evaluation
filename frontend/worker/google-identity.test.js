// @vitest-environment node
import { webcrypto } from 'node:crypto'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { createGoogleIdentityProvider } from './google-identity.js'

const audience = 'https://test.run.app'
let account, publicKey
const encode = value => btoa(JSON.stringify(value)).replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_')
const token = (aud = audience, exp = Date.now()/1000+3600) => `${encode({ alg: 'RS256' })}.${encode({ aud, exp })}.test`
const decode = part => JSON.parse(atob(part.replace(/-/g, '+').replace(/_/g, '/')))

beforeAll(async () => {
  // テスト専用の鍵をメモリ内で作る。本番の資格情報は読まない。
  const keys = await webcrypto.subtle.generateKey({ name: 'RSASSA-PKCS1-v1_5',
    modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: 'SHA-256' }, true, ['sign', 'verify'])
  publicKey = keys.publicKey
  const bytes = new Uint8Array(await webcrypto.subtle.exportKey('pkcs8', keys.privateKey))
  account = JSON.stringify({ type: 'service_account', client_email: 'test@test.iam.gserviceaccount.com',
    private_key_id: 'test-key', private_key: `-----BEGIN PRIVATE KEY-----\n${btoa(String.fromCharCode(...bytes))}\n-----END PRIVATE KEY-----` })
})
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

function setup() {
  vi.stubGlobal('crypto', webcrypto)
  const exchange = vi.fn(async () => Response.json({ id_token: token() }))
  vi.stubGlobal('fetch', exchange)
  return { exchange, get: createGoogleIdentityProvider(), env: { GCP_SERVICE_ACCOUNT_JSON: account } }
}

describe('GoogleのIDトークン取得', () => {
  it('宛先付きJWTを署名し、固定のGoogleエンドポイントで交換する', async () => {
    const { exchange, get, env } = setup()
    await get(env, audience)
    const [url, options] = exchange.mock.calls[0]
    expect(url).toBe('https://oauth2.googleapis.com/token')
    expect(options.redirect).toBe('manual')
    const assertion = options.body.get('assertion')
    const [header, payload, signature] = assertion.split('.')
    expect(decode(header)).toEqual({ alg: 'RS256', typ: 'JWT', kid: 'test-key' })
    expect(decode(payload)).toMatchObject({ iss: 'test@test.iam.gserviceaccount.com',
      aud: url, target_audience: audience })
    const sig = Uint8Array.from(atob(signature.replace(/-/g, '+').replace(/_/g, '/')), c => c.charCodeAt(0))
    expect(await webcrypto.subtle.verify('RSASSA-PKCS1-v1_5', publicKey, sig,
      new TextEncoder().encode(`${header}.${payload}`))).toBe(true)
  })
  it('同時要求と期限内の要求でトークン発行を共有する', async () => {
    const { exchange, get, env } = setup()
    const results = await Promise.all(Array.from({ length: 10 }, () => get(env, audience)))
    expect(new Set(results).size).toBe(1)
    await get(env, audience)
    expect(exchange).toHaveBeenCalledTimes(1)
  })
  it('有効期限の1分前に新しいトークンへ更新する', async () => {
    const { exchange, get, env } = setup()
    const start = Date.now()
    await get(env, audience)
    vi.spyOn(Date, 'now').mockReturnValue(start + 3550*1000)
    await get(env, audience)
    expect(exchange).toHaveBeenCalledTimes(2)
  })
  it('秘密鍵を入れ替えるとキャッシュを使わない', async () => {
    const { exchange, get, env } = setup()
    await get(env, audience)
    await get({ GCP_SERVICE_ACCOUNT_JSON: account + ' ' }, audience)
    expect(exchange).toHaveBeenCalledTimes(2)
  })
  it.each([
    ['宛先が違う', () => Response.json({ id_token: token('https://other.run.app') })],
    ['期限切れ', () => Response.json({ id_token: token(audience, Date.now()/1000-1) })],
    ['トークンなし', () => Response.json({})],
    ['交換失敗', () => new Response('sensitive-upstream-error', { status: 400 })],
    ['別URLへの転送', () => new Response(null, { status: 302, headers: { Location: 'https://other.test' } })],
  ])('%sの応答は受け付けない', async (_name, response) => {
    const { exchange, get, env } = setup()
    exchange.mockImplementation(async () => response())
    await expect(get(env, audience)).rejects.toThrow()
  })
  it('交換の失敗を永久にキャッシュせず次の要求で再試行する', async () => {
    const { exchange, get, env } = setup()
    exchange.mockResolvedValueOnce(new Response('error', { status: 500 }))
    await expect(get(env, audience)).rejects.toThrow('Google token exchange failed')
    await expect(get(env, audience)).resolves.toBeTypeOf('string')
    expect(exchange).toHaveBeenCalledTimes(2)
  })
  it('秘密鍵が未設定ならトークン交換を実行しない', async () => {
    const { exchange, get } = setup()
    await expect(get({}, audience)).rejects.toThrow('Missing service account configuration')
    expect(exchange).not.toHaveBeenCalled()
  })
})
