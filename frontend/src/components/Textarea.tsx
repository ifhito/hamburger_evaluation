import type { TextareaHTMLAttributes } from 'react'
import { CharCounter } from './CharCounter'
import styles from './field.module.css'

interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label: string
  /** 文字数のカウンター。value は現在の入力、max は GET /meta の上限(未取得なら undefined で、出さない)。 */
  counter?: { value: string; max: number | undefined }
  id: string
}

export function Textarea({ label, counter, id, className, 'aria-describedby': ariaDescribedBy, ...props }: TextareaProps) {
  const counterId = `${id}-counter`
  const describedBy = [ariaDescribedBy, counter?.max !== undefined && counterId].filter(Boolean).join(' ')
  return (
    <div className={styles.wrapper}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <textarea
        {...props}
        id={id}
        aria-describedby={describedBy || undefined}
        className={[styles.textarea, className].filter(Boolean).join(' ')}
      />
      {counter && <CharCounter id={counterId} value={counter.value} max={counter.max} />}
    </div>
  )
}
