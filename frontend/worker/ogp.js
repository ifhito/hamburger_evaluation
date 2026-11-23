// SNS クローラー(X・LINE・Discord・Slack など)向けに、詳細ページの HTML へ動的 OGP タグを埋め込む。
// Cloudflare 固有の HTMLRewriter に依存しない文字列変換にすることで、Node 環境の Vitest でも同じコードを通す。

const UUID_SEGMENT = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
const REVIEW_PATH_RE = new RegExp(`^/reviews/(${UUID_SEGMENT})$`, 'i')
const BURGER_PATH_RE = new RegExp(`^/burgers/(${UUID_SEGMENT})$`, 'i')
const SHOP_PATH_RE = new RegExp(`^/shops/(${UUID_SEGMENT})$`, 'i')

const DEFAULT_DESCRIPTION = 'お気に入りの一口を探して、ハンバーガーの楽しみを広げよう。'
const DEFAULT_OGP_IMAGE_PATH = '/ogp/default.png'
const SUMMARY_MAX_CHARS = 110

// URL パスが公開詳細ページ(/reviews/:id, /burgers/:id, /shops/:id)に一致するとき、種別と API パスを返す。
export function matchOgpTarget(pathname) {
  let match = REVIEW_PATH_RE.exec(pathname)
  if (match) return { kind: 'review', apiPath: `/reviews/${match[1]}` }
  match = BURGER_PATH_RE.exec(pathname)
  if (match) return { kind: 'burger', apiPath: `/burgers/${match[1]}` }
  match = SHOP_PATH_RE.exec(pathname)
  if (match) return { kind: 'shop', apiPath: `/shops/${match[1]}` }
  return null
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function summarizeText(text, maxChars = SUMMARY_MAX_CHARS) {
  if (typeof text !== 'string') return ''
  const normalized = text.replace(/\s+/g, ' ').trim()
  if (!normalized) return ''
  const chars = Array.from(normalized)
  if (chars.length <= maxChars) return normalized
  return `${chars.slice(0, maxChars).join('')}…`
}

// 写真の公開 URL を安全な絶対 http(s) URL に解決し、未設定や不正な形式のときは既定の OGP 画像へフォールバックする。
export function resolveOgpImageUrl(photoUrl, pageOrigin) {
  const baseOrigin = new URL(pageOrigin).origin
  if (typeof photoUrl === 'string' && photoUrl.trim() !== '') {
    const trimmed = photoUrl.trim()
    if (!trimmed.startsWith('//')) {
      try {
        const isRootRelative = trimmed.startsWith('/')
        const parsed = isRootRelative ? new URL(trimmed, baseOrigin) : new URL(trimmed)
        const sameOriginOk = !isRootRelative || parsed.origin === baseOrigin
        if (
          sameOriginOk &&
          (parsed.protocol === 'https:' || parsed.protocol === 'http:') &&
          !parsed.username &&
          !parsed.password
        ) {
          return parsed.href
        }
      } catch {
        // 不正な URL 文字列は既定画像へフォールバックする。
      }
    }
  }
  return new URL(DEFAULT_OGP_IMAGE_PATH, baseOrigin).href
}

// バックエンドの公開 API 応答(snake_case)から OGP 用のメタ情報を組み立てる。
export function buildOgpMeta(kind, data, pageUrl) {
  const canonicalUrl = `${pageUrl.origin}${pageUrl.pathname}`
  const image = resolveOgpImageUrl(data?.photo_url, pageUrl.origin)

  if (kind === 'review') {
    const burgerName = typeof data?.burger?.name === 'string' ? data.burger.name.trim() : ''
    const shopName = typeof data?.shop?.name === 'string' ? data.shop.name.trim() : ''
    const ratingText = typeof data?.rating === 'number' ? `★${data.rating}` : ''
    let subject = '🍔 ハンバーガーレビュー'
    if (burgerName && shopName) {
      subject = `🍔 ${burgerName}（${shopName}）`
    } else if (burgerName) {
      subject = `🍔 ${burgerName}`
    } else if (shopName) {
      subject = `🍔 ${shopName}`
    }
    const headline = [subject, ratingText].filter(Boolean).join(' ')
    const commentSummary = summarizeText(data?.comment)
    const authorName = typeof data?.user?.username === 'string' ? data.user.username.trim() : ''
    let description = DEFAULT_DESCRIPTION
    if (commentSummary) {
      description = authorName ? `${commentSummary}（${authorName}）` : commentSummary
    } else if (authorName) {
      description = `${authorName}さんのハンバーガーレビュー | BurgerStack`
    }
    return {
      type: 'article',
      url: canonicalUrl,
      title: `${headline} | BurgerStack`,
      description,
      image,
    }
  }

  if (kind === 'burger') {
    const burgerName = typeof data?.name === 'string' && data.name.trim() ? data.name.trim() : 'バーガー'
    const shopNames = Array.isArray(data?.shops)
      ? data.shops
          .map((s) => (typeof s?.name === 'string' ? s.name.trim() : ''))
          .filter(Boolean)
          .join('・')
      : ''
    const subject = `🍔 ${burgerName}${shopNames ? `（${shopNames}）` : ''}`
    const hasStats = typeof data?.review_count === 'number' && data.review_count > 0 && typeof data?.average_rating === 'number'
    const description = hasStats
      ? `平均評価 ★${data.average_rating.toFixed(1)}（${data.review_count}件のレビュー） — BurgerStack で口コミと写真をチェック`
      : `${burgerName}のレビュー・写真を BurgerStack でチェック`
    return {
      type: 'website',
      url: canonicalUrl,
      title: `${subject} | BurgerStack`,
      description,
      image,
    }
  }

  const shopName = typeof data?.name === 'string' && data.name.trim() ? data.name.trim() : 'ショップ'
  const hasStats = typeof data?.review_count === 'number' && data.review_count > 0 && typeof data?.average_rating === 'number'
  const description = hasStats
    ? `平均評価 ★${data.average_rating.toFixed(1)}（${data.review_count}件のレビュー） — BurgerStack でメニューと口コミをチェック`
    : `${shopName}のハンバーガーレビューを BurgerStack でチェック`
  return {
    type: 'website',
    url: canonicalUrl,
    title: `🍔 ${shopName} | BurgerStack`,
    description,
    image,
  }
}

// index.html の既定の title / OGP タグを取り除き、エスケープ済みの動的 OGP タグを </head> の直前に差し込む。
export function rewriteHtmlMeta(html, meta) {
  const headCloseIndex = html.search(/<\/head>/i)
  if (headCloseIndex === -1) return html

  const headPart = html
    .slice(0, headCloseIndex)
    .replace(/<title>[\s\S]*?<\/title>\s*/gi, '')
    .replace(/<meta\s+(?:name|property)=["'](?:description|og:[^"']+|twitter:[^"']+)["'][^>]*>\s*/gi, '')

  const injected = [
    `    <title>${escapeHtml(meta.title)}</title>`,
    `    <meta name="description" content="${escapeHtml(meta.description)}" />`,
    `    <meta property="og:site_name" content="BurgerStack" />`,
    `    <meta property="og:type" content="${escapeHtml(meta.type)}" />`,
    `    <meta property="og:url" content="${escapeHtml(meta.url)}" />`,
    `    <meta property="og:title" content="${escapeHtml(meta.title)}" />`,
    `    <meta property="og:description" content="${escapeHtml(meta.description)}" />`,
    `    <meta property="og:image" content="${escapeHtml(meta.image)}" />`,
    `    <meta name="twitter:card" content="summary_large_image" />`,
    `    <meta name="twitter:title" content="${escapeHtml(meta.title)}" />`,
    `    <meta name="twitter:description" content="${escapeHtml(meta.description)}" />`,
    `    <meta name="twitter:image" content="${escapeHtml(meta.image)}" />`,
    '',
  ].join('\n')

  return `${headPart}${injected}${html.slice(headCloseIndex)}`
}
