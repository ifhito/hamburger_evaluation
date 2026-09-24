const TOKEN_URL = 'https://oauth2.googleapis.com/token'
const encoder = new TextEncoder()

function base64url(bytes) {
  return btoa(String.fromCharCode(...bytes)).replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_')
}

function encodeJSON(value) {
  return base64url(encoder.encode(JSON.stringify(value)))
}

// Google の HTTPS 応答から受け取ったトークンの有効期限を確認する。
// ブラウザなど外部の利用者が送ったトークンの検証には使わない。
function tokenClaims(token) {
  const parts = token.split('.')
  if (parts.length !== 3) throw new Error('Invalid Google ID token')
  const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/')
  return JSON.parse(atob(payload.padEnd(Math.ceil(payload.length / 4) * 4, '=')))
}

// キャッシュは isolate 内だけに置き、鍵・宛先が変わったら共有しない。
// 更新中の Promise も共有し、同時要求ごとのトークン発行を避ける。
export function createGoogleIdentityProvider() {
  let cached
  let pending
  let credentials
  let audience

  async function issue(rawCredentials, targetAudience) {
    const account = JSON.parse(rawCredentials)
    if (account.type !== 'service_account' || typeof account.private_key !== 'string' ||
        typeof account.client_email !== 'string' || !account.client_email.endsWith('.iam.gserviceaccount.com')) {
      throw new Error('Invalid service account configuration')
    }
    const pem = account.private_key.replace(/-----BEGIN PRIVATE KEY-----|-----END PRIVATE KEY-----|\s/g, '')
    const key = await crypto.subtle.importKey('pkcs8', Uint8Array.from(atob(pem), c => c.charCodeAt(0)),
      { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' }, false, ['sign'])
    const now = Math.floor(Date.now() / 1000)
    const unsigned = `${encodeJSON({ alg: 'RS256', typ: 'JWT', kid: account.private_key_id })}.${encodeJSON({
      iss: account.client_email, sub: account.client_email, aud: TOKEN_URL,
      iat: now, exp: now + 3600, target_audience: targetAudience,
    })}`
    const signature = await crypto.subtle.sign('RSASSA-PKCS1-v1_5', key, encoder.encode(unsigned))
    const response = await fetch(TOKEN_URL, {
      // Workers は redirect: 'error' に非対応。3xx は response.ok で拒否する。
      method: 'POST', redirect: 'manual', signal: AbortSignal.timeout(10000),
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'urn:ietf:params:oauth:grant-type:jwt-bearer',
        assertion: `${unsigned}.${base64url(new Uint8Array(signature))}` }),
    })
    if (!response.ok) throw new Error('Google token exchange failed')
    const result = await response.json()
    if (typeof result.id_token !== 'string') throw new Error('Missing Google ID token')
    const claims = tokenClaims(result.id_token)
    if (claims.aud !== targetAudience || !Number.isFinite(claims.exp) || claims.exp <= Date.now() / 1000 + 60) {
      throw new Error('Invalid Google token audience or expiry')
    }
    return { token: result.id_token, expiresAt: Math.min(claims.exp, now + 3600) }
  }

  return async function getIDToken(env, targetAudience) {
    const raw = env.GCP_SERVICE_ACCOUNT_JSON
    if (typeof raw !== 'string' || !raw) throw new Error('Missing service account configuration')
    if (credentials !== raw || audience !== targetAudience) {
      credentials = raw
      audience = targetAudience
      cached = undefined
      pending = undefined
    }
    if (cached && cached.expiresAt > Date.now() / 1000 + 60) return cached.token
    if (!pending) {
      const request = issue(raw, targetAudience).then(value => {
        if (pending === request) {
          cached = value
          pending = undefined
        }
        return value
      }, error => {
        if (pending === request) pending = undefined
        throw error
      })
      pending = request
    }
    return (await pending).token
  }
}
