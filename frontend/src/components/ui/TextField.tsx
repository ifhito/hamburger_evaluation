import type { InputHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { CharCounter } from '../CharCounter'
import styles from './field.module.css'

interface Common {
  id: string
  label: string
  // 任意の項目に付ける説明(例「(任意)」)。ラベルの隣に薄く出す。
  optional?: string
  // 入力欄の下に常時出す説明文。入力欄に aria-describedby で結び付ける。
  hint?: string
  // 文字数のカウンター。max は GET /meta の上限(未取得なら undefined で、出さない)。
  counter?: { value: string; max: number | undefined }
}

function describedBy(id: string, hint: string | undefined, counter: Common['counter']) {
  return [hint && `${id}-hint`, counter?.max !== undefined && `${id}-counter`].filter(Boolean).join(' ') || undefined
}

function Frame({ id, label, optional, hint, counter, children }: Common & { children: React.ReactNode }) {
  return (
    <div className={styles.field}>
      <label htmlFor={id} className={styles.label}>
        {label}
        {optional && <span className={styles.optional}> {optional}</span>}
      </label>
      {children}
      {hint && (
        <span id={`${id}-hint`} className={styles.hint}>
          {hint}
        </span>
      )}
      {counter && <CharCounter id={`${id}-counter`} value={counter.value} max={counter.max} />}
    </div>
  )
}

// 1 行の入力欄(ラベル・説明・文字数のカウンターつき)。エラーは、画面の上に Alert で出す(API の文言をそのまま)。
export function TextField({ id, label, optional, hint, counter, ...props }: Common & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <Frame id={id} label={label} optional={optional} hint={hint} counter={counter}>
      <input id={id} aria-describedby={describedBy(id, hint, counter)} {...props} className={styles.input} />
    </Frame>
  )
}

// 複数行の入力欄。
export function TextArea({ id, label, optional, hint, counter, ...props }: Common & TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <Frame id={id} label={label} optional={optional} hint={hint} counter={counter}>
      <textarea id={id} aria-describedby={describedBy(id, hint, counter)} {...props} className={`${styles.input} ${styles.area}`} />
    </Frame>
  )
}
