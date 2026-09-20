import type { InputHTMLAttributes } from 'react'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  error?: string
  /** 入力欄の下に常時表示する説明文。input に aria-describedby で紐付ける。 */
  hint?: string
  id: string
}

export function Input({ label, error, hint, id, style, ...props }: InputProps) {
  const hintId = `${id}-hint`
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <label htmlFor={id} style={{ fontSize: '0.875rem', fontWeight: 500 }}>
        {label}
      </label>
      <input
        id={id}
        aria-describedby={hint ? hintId : undefined}
        {...props}
        style={{
          padding: '8px 12px',
          border: `1px solid ${error ? 'var(--color-error)' : 'var(--color-border)'}`,
          borderRadius: 'var(--radius)',
          fontSize: '0.875rem',
          outline: 'none',
          width: '100%',
          ...style,
        }}
      />
      {hint && (
        <span id={hintId} style={{ fontSize: '0.75rem', color: 'var(--color-text-muted)' }}>{hint}</span>
      )}
      {error && (
        <span style={{ fontSize: '0.75rem', color: 'var(--color-error)' }}>{error}</span>
      )}
    </div>
  )
}
