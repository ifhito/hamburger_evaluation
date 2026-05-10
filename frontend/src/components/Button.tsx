import type { ButtonHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import styles from './button.module.css'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger'
  isLoading?: boolean
}

export function Button({
  variant = 'primary',
  isLoading = false,
  disabled,
  children,
  className,
  ...props
}: ButtonProps) {
  const { t } = useTranslation()
  const isDisabled = disabled ?? isLoading
  const cls = [
    styles.btn,
    styles[variant],
    isDisabled ? styles.disabled : '',
    className ?? '',
  ].filter(Boolean).join(' ')

  return (
    <button {...props} disabled={isDisabled} className={cls}>
      {isLoading ? t('common.loading') : children}
    </button>
  )
}
