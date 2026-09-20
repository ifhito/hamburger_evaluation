import type { InputHTMLAttributes } from 'react'
import styles from './field.module.css'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  error?: string
  id: string
}

export function Input({ label, error, id, ...props }: InputProps) {
  return (
    <div className={styles.wrapper}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <input
        id={id}
        {...props}
        className={`${styles.input}${error ? ` ${styles.fieldError}` : ''}`}
      />
      {error && <span className={styles.errorText}>{error}</span>}
    </div>
  )
}
