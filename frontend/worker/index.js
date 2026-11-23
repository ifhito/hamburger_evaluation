// Cloudflare Workers(Static Assets)用。静的ファイルを配信しつつ、
// /api/* と OAuth の発見用 URL を、IAM 認証付きで API へ転送し、
// 公開詳細ページ(/reviews/:id, /burgers/:id, /shops/:id)の HTML には動的 OGP タグを注入する。
//
// API のオリジンは環境変数 API_ORIGIN、秘密鍵は Worker の Secret から読む。
import { createGoogleIdentityProvider } from './google-identity.js'
import { buildOgpMeta, matchOgpTarget, rewriteHtmlMeta } from './ogp.js'

const getIDToken = createGoogleIdentityProvider()
const OGP_FETCH_TIMEOUT_MS = 2500

function parseAllowedRunOrigin(raw, label) {
  const parsed = new URL(raw)
  if (
    parsed.protocol !== 'https:' ||
    parsed.username ||
    parsed.password ||
    parsed.pathname !== '/' ||
    parsed.search ||
    parsed.hash ||
    !parsed.hostname.endsWith('.run.app')
  ) {
    throw new Error(`Invalid ${label}`)
  }
  return parsed
}

async function resolveUpstreamAuth(env) {
  const origin = parseAllowedRunOrigin(env.API_ORIGIN, 'API origin')
  const audience = parseAllowedRunOrigin(env.CLOUD_RUN_AUDIENCE || origin.origin, 'Cloud Run audience')
  const idToken = await getIDToken(env, audience.origin)
  return { apiOrigin: origin.origin, idToken }
}

// 公開 API から匿名で OGP 用の JSON を取得する。審査待ち店舗など匿名では 404 になる対象や、
// API 側のタイムアウト・障害時は null を返し、SPA の HTML 配信自体は止めない(fail-open)。
// 閲覧者の Authorization や Cookie は一切渡さず、公開情報だけを OGP に載せる。
async function fetchOgpData(env, apiPath) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), OGP_FETCH_TIMEOUT_MS)
  try {
    const { apiOrigin, idToken } = await resolveUpstreamAuth(env)
    const target = new URL(apiOrigin)
    target.pathname = apiPath
    const req = new Request(target, {
      method: 'GET',
      headers: {
        Accept: 'application/json',
        'X-Serverless-Authorization': `Bearer ${idToken}`,
      },
    })
    const res = await fetch(req, { redirect: 'manual', signal: controller.signal })
    if (!res.ok) {
      await res.body?.cancel?.()
      return null
    }
    const contentType = res.headers.get('content-type') || ''
    if (!contentType.toLowerCase().includes('application/json')) {
      await res.body?.cancel?.()
      return null
    }
    return await res.json()
  } catch {
    return null
  } finally {
    clearTimeout(timer)
  }
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url)
    const discovery = {
      '/.well-known/oauth-authorization-server/api': '/.well-known/oauth-authorization-server',
      '/.well-known/oauth-authorization-server': '/.well-known/oauth-authorization-server',
      '/.well-known/oauth-protected-resource/api/mcp': '/.well-known/oauth-protected-resource/api/mcp',
      '/.well-known/oauth-protected-resource': '/.well-known/oauth-protected-resource',
    }
    const discoveryPath = Object.hasOwn(discovery, url.pathname) ? discovery[url.pathname] : undefined
    if (url.pathname.startsWith('/api/') || discoveryPath) {
      // /api を取り除いて API へ渡す(Vite の dev proxy と同じ規則)。
      let target, idToken
      try {
        const auth = await resolveUpstreamAuth(env)
        idToken = auth.idToken
        // 先頭が // のパスも別ホストとして解釈させない。
        target = new URL(auth.apiOrigin)
        target.pathname = discoveryPath || url.pathname.slice(4)
        target.search = url.search
      } catch {
        // Google の応答や鍵を、ログ・ブラウザ・MCP のクライアントに出さない。
        return new Response('API authentication unavailable', { status: 503, headers: { 'Cache-Control': 'no-store' } })
      }
      const forwarded = new Request(target, request)
      forwarded.headers.set('X-Serverless-Authorization', `Bearer ${idToken}`)
      // redirect: "manual" が要る。既定では fetch が 3xx を Worker の中で
      // 追いかけてしまい、リダイレクト先の中身を 200 として返す。そうなると
      // ブラウザの URL が変わらず、Google ログインのような 302 を前提にした
      // 流れが壊れる(アプリが知らない /api/... のまま描画され 404 になる)。
      return fetch(forwarded, { redirect: 'manual' })
    }

    if (request.method === 'GET') {
      const ogpTarget = matchOgpTarget(url.pathname)
      if (ogpTarget) {
        // クローラー(Accept: */* など Sec-Fetch-Mode: navigate を持たない要求)でも確実に index.html を得られ、
        // かつ If-None-Match による 304 Not Modified(ボディなし応答)を発生させないよう、'/' を無条件で取得する。
        const shellRequest = new Request(new URL('/', url.origin), {
          method: 'GET',
          headers: { Accept: 'text/html' },
        })
        const [assetRes, ogpData] = await Promise.all([
          env.ASSETS.fetch(shellRequest),
          fetchOgpData(env, ogpTarget.apiPath),
        ])
        const contentType = assetRes.headers.get('content-type') || ''
        if (assetRes.status !== 200 || !contentType.toLowerCase().includes('text/html')) {
          return assetRes
        }
        const headers = new Headers(assetRes.headers)
        headers.delete('content-length')
        headers.delete('etag')
        headers.set('Cache-Control', 'public, max-age=0, must-revalidate')
        if (!ogpData) {
          return new Response(assetRes.body, {
            status: assetRes.status,
            statusText: assetRes.statusText,
            headers,
          })
        }
        const html = await assetRes.text()
        const meta = buildOgpMeta(ogpTarget.kind, ogpData, url)
        const rewritten = rewriteHtmlMeta(html, meta)
        return new Response(rewritten, {
          status: assetRes.status,
          statusText: assetRes.statusText,
          headers,
        })
      }
    }

    return env.ASSETS.fetch(request)
  },
}
