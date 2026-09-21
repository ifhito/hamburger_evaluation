import type { HTMLAttributes } from 'react'
import styles from './card.module.css'

interface Props extends HTMLAttributes<HTMLDivElement> {
  // 強調する(いま見ているレビューなど): 黒い太い枠 + 薄い灰色の面。
  current?: boolean
  // 内側に余白を付ける。写真を端まで見せるカードは false にして、中身が余白を持つ。
  padded?: boolean
}

export function Card({ current = false, padded = true, className, ...props }: Props) {
  const cls = [styles.card, current ? styles.current : '', padded ? styles.padded : '', className ?? ''].filter(Boolean).join(' ')
  return <div {...props} className={cls} />
}
