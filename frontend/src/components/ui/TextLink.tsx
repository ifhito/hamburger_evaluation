import { Link, type LinkProps } from 'react-router-dom'
import styles from './textLink.module.css'

// 下線つきの黒い文字のリンク(「← プロフィールに戻る」など)。色でなく、下線でリンクと分かる。
export function TextLink({ className, ...props }: LinkProps) {
  return <Link {...props} className={[styles.textLink, className].filter(Boolean).join(' ')} />
}
