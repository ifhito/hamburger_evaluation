import type { InputHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import { countChars } from '../../lib/countChars'
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

// 文字数「いまの文字数 / 上限」。上限を超えても入力も送信も止めない(判定は backend の 422)。超えている間は、色だけでなく
// 「Too long」の文字でも知らせる。読み上げには、超えたときと戻ったときの 2 回だけ、見えない role="status" で伝える。
function Counter({ id, value, max }: { id: string; value: string; max: number }) {
  const { t } = useTranslation()
  const count = countChars(value)
  const over = count > max
  return (
    <div className={styles.counterRow}>
      <span id={id} className={over ? `${styles.counter} ${styles.over}` : styles.counter}>
        {count} / {max}
        {over && ` ${t('common.charCounter.tooLong')}`}
      </span>
      <span role="status" className={styles.srOnly}>
        {over ? t('common.charCounter.overAnnounce', { max }) : t('common.charCounter.withinAnnounce')}
      </span>
    </div>
  )
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
      {counter?.max !== undefined && <Counter id={`${id}-counter`} value={counter.value} max={counter.max} />}
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

// 複数行の入力欄。short は、高さを低くする(却下の理由など、短い文を書く欄)。
export function TextArea({ id, label, optional, hint, counter, short, ...props }: Common & TextareaHTMLAttributes<HTMLTextAreaElement> & { short?: boolean }) {
  return (
    <Frame id={id} label={label} optional={optional} hint={hint} counter={counter}>
      <textarea
        id={id}
        aria-describedby={describedBy(id, hint, counter)}
        {...props}
        className={[styles.input, styles.area, short ? styles.short : ''].filter(Boolean).join(' ')}
      />
    </Frame>
  )
}
