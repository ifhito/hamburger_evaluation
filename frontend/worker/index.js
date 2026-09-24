// Cloudflare Workers(Static Assets)用。静的ファイルを配信しつつ、
// /api/* と OAuth の発見用 URL を、IAM 認証付きで API へ転送する。
//
// API のオリジンは環境変数 API_ORIGIN、秘密鍵は Worker の Secret から読む。
import { createGoogleIdentityProvider } from './google-identity.js'

const getIDToken = createGoogleIdentityProvider()

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const discovery = {
      '/.well-known/oauth-authorization-server/api': '/.well-known/oauth-authorization-server',
      '/.well-known/oauth-authorization-server': '/.well-known/oauth-authorization-server',
      '/.well-known/oauth-protected-resource/api/mcp': '/.well-known/oauth-protected-resource/api/mcp',
      '/.well-known/oauth-protected-resource': '/.well-known/oauth-protected-resource',
    }
    const discoveryPath = Object.hasOwn(discovery, url.pathname) ? discovery[url.pathname] : undefined
    if (url.pathname.startsWith("/api/") || discoveryPath) {
      // /api を取り除いて API へ渡す(Vite の dev proxy と同じ規則)。
      let target, idToken
      try {
        const origin = new URL(env.API_ORIGIN)
        if (origin.protocol !== 'https:' || origin.username || origin.password ||
            origin.pathname !== '/' || origin.search || origin.hash || !origin.hostname.endsWith('.run.app')) {
          throw new Error('Invalid API origin')
        }
        // 先頭が // のパスも別ホストとして解釈させない。
        target = new URL(origin.origin)
        target.pathname = discoveryPath || url.pathname.slice(4)
        target.search = url.search
        const audience = new URL(env.CLOUD_RUN_AUDIENCE || origin.origin)
        if (audience.protocol !== 'https:' || audience.username || audience.password ||
            audience.pathname !== '/' || audience.search || audience.hash || !audience.hostname.endsWith('.run.app')) {
          throw new Error('Invalid Cloud Run audience')
        }
        idToken = await getIDToken(env, audience.origin)
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
      return fetch(forwarded, { redirect: "manual" });
    }
    return env.ASSETS.fetch(request);
  },
};
