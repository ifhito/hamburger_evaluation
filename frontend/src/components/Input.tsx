import type { InputHTMLAttributes } from 'react'
import styles from './field.module.css'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  /** 入力欄の下に常時表示する説明文。input に aria-describedby で紐付ける。 */
  hint?: string
  id: string
}

export function Input({ label, hint, id, ...props }: InputProps) {
  const hintId = `${id}-hint`
  return (
    <div className={styles.wrapper}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <input
        id={id}
        aria-describedby={hint ? hintId : undefined}
        {...props}
        className={styles.input}
      />
      {hint && <span id={hintId} className={styles.hintText}>{hint}</span>}
    </div>
  )
}
