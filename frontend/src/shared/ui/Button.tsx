import type { ButtonHTMLAttributes } from 'react'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger'
  isLoading?: boolean
}

const inlineStyles: Record<string, React.CSSProperties> = {
  primary: { background: 'var(--color-primary)', color: '#fff', border: 'none' },
  secondary: { background: 'var(--color-secondary-bg)', color: 'var(--color-text)', border: 'none' },
  danger: { background: 'var(--color-danger)', color: '#fff', border: 'none' },
}

export function Button({
  variant = 'primary',
  isLoading = false,
  disabled,
  children,
  style,
  ...props
}: ButtonProps) {
  return (
    <button
      {...props}
      disabled={disabled ?? isLoading}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '8px 16px',
        borderRadius: 'var(--radius)',
        fontWeight: 500,
        fontSize: '0.875rem',
        cursor: disabled || isLoading ? 'not-allowed' : 'pointer',
        opacity: disabled || isLoading ? 0.6 : 1,
        transition: 'background-color 0.15s',
        ...inlineStyles[variant],
        ...style,
      }}
    >
      {isLoading ? 'Loading…' : children}
    </button>
  )
}
