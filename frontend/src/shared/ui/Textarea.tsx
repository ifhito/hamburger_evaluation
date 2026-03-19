import type { TextareaHTMLAttributes } from 'react'

interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label: string
  error?: string
  id: string
}

export function Textarea({ label, error, id, style, ...props }: TextareaProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <label htmlFor={id} style={{ fontSize: '0.875rem', fontWeight: 500 }}>
        {label}
      </label>
      <textarea
        id={id}
        {...props}
        style={{
          padding: '8px 12px',
          border: `1px solid ${error ? 'var(--color-error)' : 'var(--color-border)'}`,
          borderRadius: 'var(--radius)',
          fontSize: '0.875rem',
          outline: 'none',
          width: '100%',
          resize: 'vertical',
          minHeight: 100,
          ...style,
        }}
      />
      {error && (
        <span style={{ fontSize: '0.75rem', color: 'var(--color-error)' }}>{error}</span>
      )}
    </div>
  )
}
