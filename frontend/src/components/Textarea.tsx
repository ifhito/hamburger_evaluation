import type { TextareaHTMLAttributes } from 'react'
import styles from './field.module.css'

interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label: string
  id: string
}

export function Textarea({ label, id, ...props }: TextareaProps) {
  return (
    <div className={styles.wrapper}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <textarea id={id} {...props} className={styles.textarea} />
    </div>
  )
}
