import type { InputHTMLAttributes } from 'react'
import { CharCounter } from './CharCounter'
import styles from './field.module.css'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  /** 入力欄の下に常時表示する説明文。input に aria-describedby で紐付ける。 */
  hint?: string
  /** 文字数のカウンター。value は現在の入力、max は GET /meta の上限(未取得なら undefined で、出さない)。 */
  counter?: { value: string; max: number | undefined }
  id: string
}

export function Input({ label, hint, counter, id, ...props }: InputProps) {
  const hintId = `${id}-hint`
  const counterId = `${id}-counter`
  const describedBy = [hint && hintId, counter?.max !== undefined && counterId].filter(Boolean).join(' ')
  return (
    <div className={styles.wrapper}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <input
        id={id}
        aria-describedby={describedBy || undefined}
        {...props}
        className={styles.input}
      />
      {hint && <span id={hintId} className={styles.hintText}>{hint}</span>}
      {counter && <CharCounter id={counterId} value={counter.value} max={counter.max} />}
    </div>
  )
}
