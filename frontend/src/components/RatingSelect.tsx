import { useTranslation } from 'react-i18next'
import styles from './field.module.css'

interface RatingSelectProps {
  value: number
  onChange: (v: number) => void
  error?: string
}

export function RatingSelect({ value, onChange, error }: RatingSelectProps) {
  const { t } = useTranslation()

  return (
    <div className={styles.wrapper}>
      <label className={styles.label}>{t('common.rating')}</label>
      <select
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className={`${styles.select}${error ? ` ${styles.fieldError}` : ''}`}
      >
        {[1, 2, 3, 4, 5].map((n) => (
          <option key={n} value={n}>
            {'★'.repeat(n)} ({n})
          </option>
        ))}
      </select>
      {error && <span className={styles.errorText}>{error}</span>}
    </div>
  )
}
