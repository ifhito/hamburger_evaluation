import { MARK, MARK_SMALL } from './logoMarks'
import styles from './logo.module.css'

// design/README.md の「ロゴの使い方」に従い、記号の高さがこの値(px)未満のときは、波を省いた小さい版を使う。
export const SMALL_MARK_BELOW_PX = 24

interface LogoProps {
  // ブランド名(例: BurgerStack)。大文字の切れ目で、前半を太字、後半を標準の太さにする。
  name: string
  // 記号の高さ(px)。
  size?: number
  // 白い面の上は 'on-light'(深い黄)、暗い面の上は 'on-dark'(今のマスタード黄)。
  variant?: 'on-light' | 'on-dark'
}

function splitName(name: string): [string, string] {
  const i = name.slice(1).search(/[A-Z]/) + 1
  return i > 0 ? [name.slice(0, i), name.slice(i)] : [name, '']
}

export function Logo({ name, size = 32, variant = 'on-light' }: LogoProps) {
  const mark = size < SMALL_MARK_BELOW_PX ? MARK_SMALL : MARK
  const [, , w, h] = mark.viewBox.split(' ').map(Number)
  const [first, rest] = splitName(name)

  return (
    <span className={variant === 'on-dark' ? `${styles.logo} ${styles.onDark}` : styles.logo}>
      <svg
        className={styles.mark}
        viewBox={mark.viewBox}
        width={Math.round((size * w) / h)}
        height={size}
        aria-hidden="true"
        focusable="false"
        data-mark={mark === MARK_SMALL ? 'small' : 'normal'}
      >
        <path d={mark.path} strokeWidth={mark.strokeWidth} />
      </svg>
      <span className={styles.text}>
        <span className={styles.first}>{first}</span>
        {rest}
      </span>
    </span>
  )
}
