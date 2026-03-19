interface ErrorMessageProps {
  message: string | string[]
}

export function ErrorMessage({ message }: ErrorMessageProps) {
  const messages = Array.isArray(message) ? message : [message]
  return (
    <div
      role="alert"
      style={{
        background: '#fee2e2',
        border: '1px solid #fca5a5',
        borderRadius: 'var(--radius)',
        padding: '8px 12px',
        color: 'var(--color-error)',
        fontSize: '0.875rem',
      }}
    >
      {messages.length === 1 ? (
        <p>{messages[0]}</p>
      ) : (
        <ul style={{ paddingLeft: '1.25rem', margin: 0 }}>
          {messages.map((m, i) => (
            <li key={i}>{m}</li>
          ))}
        </ul>
      )}
    </div>
  )
}
