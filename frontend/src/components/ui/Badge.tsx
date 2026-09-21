import type { ReactNode } from 'react'
import styles from './badge.module.css'

interface Props {
  // outline: 白い面 + 黒い枠(審査待ち・却下) / accent: 黄の面 + 黒い文字(公開中・表示中のレビュー)
  tone?: 'outline' | 'accent'
  children: ReactNode
}

// 状態を文字で見せる札。色だけに頼らず、必ず状態の文字を出す。どの状態にどちらの見た目を使うかは、デザインの各画面のとおり
// (呼び出し側が決める。状態の判断は API が返した値による)。
export function Badge({ tone = 'outline', children }: Props) {
  return <span className={tone === 'accent' ? `${styles.badge} ${styles.accent}` : styles.badge}>{children}</span>
}
