import type { HTMLAttributes } from 'react'
import styles from './card.module.css'

interface Props extends HTMLAttributes<HTMLElement> {
  // 強調する(いま見ているレビューなど): 黒い太い枠 + 薄い灰色の面。
  current?: boolean
  // 内側に余白を付ける。写真を端まで見せるカードは false にして、中身が余白を持つ。
  // 枠からはみ出す部分は隠すので、端に接する操作できる部品は、フォーカスの枠が切れないよう、内側に余白を取ること。
  padded?: boolean
}

// デザイン(design/redesign/review-detail.html の .ritem・reviews.html の .rcard)は、この箱を <article> にしている
// (レビューなど、繰り返しの独立した項目としての意味を持たせるため。<div> ではない)。
export function Card({ current = false, padded = true, className, ...props }: Props) {
  const cls = [styles.card, current ? styles.current : '', padded ? styles.padded : '', className ?? ''].filter(Boolean).join(' ')
  return <article {...props} aria-current={current || undefined} className={cls} />
}
