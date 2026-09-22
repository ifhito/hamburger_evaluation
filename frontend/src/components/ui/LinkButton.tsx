import { Link, type LinkProps } from 'react-router-dom'
import styles from './linkButton.module.css'

interface Props extends LinkProps {
  // primary: 黄の面(「ショップを追加」「レビューを書く」など、リンクで別画面へ移る主の操作) / secondary: 白い面 + 枠(既定)。
  variant?: 'primary' | 'secondary'
}

// 別のページへ移る、ボタンと同じ見た目のリンク。ボタンをリンクの中に入れない(押せるものが二重になり、Tab の停止も 2 回になるため)。
export function LinkButton({ variant = 'secondary', className, ...props }: Props) {
  const base = variant === 'primary' ? styles.primary : styles.linkButton
  const cls = [base, className].filter(Boolean).join(' ')
  return <Link {...props} className={cls} />
}
