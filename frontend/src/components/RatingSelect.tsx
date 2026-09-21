import { useTranslation } from 'react-i18next'
import styles from './field.module.css'

interface RatingSelectProps {
  value: number
  onChange: (v: number) => void
  // 選択肢の範囲。backend の domain の値(GET /meta)を、呼び出し側が渡す。
  min: number
  max: number
}

export function RatingSelect({ value, onChange, min, max }: RatingSelectProps) {
  const { t } = useTranslation()

  return (
    <div className={styles.wrapper}>
      <label className={styles.label}>{t('common.rating')}</label>
      <select
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className={styles.select}
      >
        {Array.from({ length: max - min + 1 }, (_, i) => min + i).map((n) => (
          <option key={n} value={n}>
            {'★'.repeat(n)} ({n})
          </option>
        ))}
      </select>
    </div>
  )
}
