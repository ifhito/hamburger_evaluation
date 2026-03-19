interface RatingSelectProps {
  value: number
  onChange: (v: number) => void
  error?: string
}

export function RatingSelect({ value, onChange, error }: RatingSelectProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <label style={{ fontSize: '0.875rem', fontWeight: 500 }}>Rating</label>
      <select
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        style={{
          padding: '8px 12px',
          border: `1px solid ${error ? 'var(--color-error)' : 'var(--color-border)'}`,
          borderRadius: 'var(--radius)',
          fontSize: '0.875rem',
          background: '#fff',
          cursor: 'pointer',
        }}
      >
        {[1, 2, 3, 4, 5].map((n) => (
          <option key={n} value={n}>
            {'★'.repeat(n)} ({n})
          </option>
        ))}
      </select>
      {error && (
        <span style={{ fontSize: '0.75rem', color: 'var(--color-error)' }}>{error}</span>
      )}
    </div>
  )
}
