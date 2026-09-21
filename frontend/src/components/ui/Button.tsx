import type { ButtonHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import styles from './button.module.css'

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  // primary: 黄の面 + 黒い文字 / secondary: 白い面 + 枠 / danger: 白い面 + 赤の枠 + 赤い文字(削除だけに使う)
  // dangerSolid: 赤の面 + 白い文字(削除・解除の確認の「削除する」) / dark: 黒い面 + 白い文字(却下の確定)
  variant?: 'primary' | 'secondary' | 'danger' | 'dangerSolid' | 'dark'
  // 送信中。押せなくなり、loadingLabel(なければ「Loading…」)を出す。
  isLoading?: boolean
  loadingLabel?: string
  // 幅いっぱいにする。
  block?: boolean
  // 左右の余白を広げる(画面の主なボタン。デザインの .wide)。
  wide?: boolean
}

// リデザインのボタン。無効・送信中は、灰色の面 + 薄い文字で、押せないことを見た目でも示す(削除の送信中は、文字だけ赤)。
export function Button({ variant = 'primary', isLoading = false, loadingLabel, block = false, wide = false, disabled, children, className, ...props }: Props) {
  const { t } = useTranslation()
  const off = disabled || isLoading
  const cls = [
    styles.btn,
    off ? (variant === 'danger' ? styles.offDanger : styles.off) : styles[variant],
    block ? styles.block : '',
    wide ? styles.wide : '',
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ')
  return (
    <button type="button" {...props} disabled={off} aria-busy={isLoading || undefined} className={cls}>
      {isLoading ? (loadingLabel ?? t('common.loading')) : children}
    </button>
  )
}
