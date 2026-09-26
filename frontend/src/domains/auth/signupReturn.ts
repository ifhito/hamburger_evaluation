import { appPathOrNull } from '../../app/router/returnTo'

const KEY = 'burgerstack:signup-return'
// 同じブラウザで確認メールを別タブで開いても、投稿・マイページへ戻せるよう短時間だけ保存する。
const MAX_AGE = 30 * 60 * 1000

export function rememberSignupReturn(path: string | null) {
  try {
    localStorage.removeItem(KEY)
    const safe = appPathOrNull(path)
    if (safe && (safe === '/record' || safe === '/me' || safe.startsWith('/reviews/new?'))) {
      localStorage.setItem(KEY, JSON.stringify({ path: safe, createdAt: Date.now() }))
    }
  } catch { /* 保存を許可しないブラウザでも登録は続ける。 */ }
}

export function consumeSignupReturn(): string | null {
  try {
    const raw = localStorage.getItem(KEY)
    localStorage.removeItem(KEY)
    if (!raw) return null
    const stored = JSON.parse(raw) as { path?: string; createdAt?: number }
    if (typeof stored.createdAt !== 'number' || Date.now() - stored.createdAt > MAX_AGE || stored.createdAt > Date.now()) return null
    return appPathOrNull(stored.path)
  } catch { return null }
}
