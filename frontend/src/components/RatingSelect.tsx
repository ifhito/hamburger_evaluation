import { useTranslation } from 'react-i18next'
import { RATING_MAX } from '../lib/rating'
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
        {Array.from({ length: RATING_MAX }, (_, i) => i + 1).map((n) => (
          <option key={n} value={n}>
            {'★'.repeat(n)} ({n})
          </option>
        ))}
      </select>
      {error && <span className={styles.errorText}>{error}</span>}
    </div>
  )
}
