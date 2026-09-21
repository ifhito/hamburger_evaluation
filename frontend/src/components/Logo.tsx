import markSvg from '../assets/logo/burgerstack-mark.svg?raw'
import styles from './logo.module.css'

// 記号(1 本の連続した線)。元は design/assets/logo/burgerstack-mark.svg で、src/assets/logo/ のファイルは、その写し
// (logoAssets.test.ts が、内容の一致を確かめる)。path を .ts に書き写さないために、SVG から読む。
// 記号は、どの大きさでも、この 1 種類(波つき)を使う。
function readMark(svg: string) {
  const path = /<path d="([^"]+)"/.exec(svg)?.[1]
  const viewBox = /viewBox="([^"]+)"/.exec(svg)?.[1]
  const strokeWidth = /stroke-width="([^"]+)"/.exec(svg)?.[1]
  if (!path || !viewBox || !strokeWidth) throw new Error('ロゴの SVG から、path・viewBox・線の太さを読めない')
  const [, , w, h] = viewBox.split(' ').map(Number)
  return { path, viewBox, strokeWidth: Number(strokeWidth), aspect: w / h }
}

const MARK = readMark(markSvg)

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
  const [first, rest] = splitName(name)

  return (
    <span className={variant === 'on-dark' ? `${styles.logo} ${styles.onDark}` : styles.logo}>
      <svg
        className={styles.mark}
        viewBox={MARK.viewBox}
        width={Math.round(size * MARK.aspect)}
        height={size}
        aria-hidden="true"
        focusable="false"
      >
        <path d={MARK.path} strokeWidth={MARK.strokeWidth} />
      </svg>
      <span className={styles.text}>
        <span className={styles.first}>{first}</span>
        {rest}
      </span>
    </span>
  )
}
