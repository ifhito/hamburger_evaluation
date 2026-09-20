import styles from './errorMessage.module.css'

interface ErrorMessageProps {
  message: string | string[]
}

export function ErrorMessage({ message }: ErrorMessageProps) {
  const messages = Array.isArray(message) ? message : [message]
  return (
    <div role="alert" className={styles.container}>
      {messages.length === 1 ? (
        <p>{messages[0]}</p>
      ) : (
        <ul className={styles.list}>
          {messages.map((m, i) => (
            <li key={i}>{m}</li>
          ))}
        </ul>
      )}
    </div>
  )
}
