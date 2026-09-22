import { Link, type LinkProps } from 'react-router-dom'
import styles from './linkButton.module.css'

// 別のページへ移る、副のボタンと同じ見た目のリンク。ボタンをリンクの中に入れない(押せるものが二重になり、Tab の停止も 2 回になるため)。
export function LinkButton({ className, ...props }: LinkProps) {
  return <Link {...props} className={[styles.linkButton, className].filter(Boolean).join(' ')} />
}
