import { useTranslation } from 'react-i18next'
import { RatingBurgerIcon } from './RatingBurger'
import styles from './ratingInput.module.css'

interface Props {
  name: string
  label: string
  // 選んでいる値。まだ選んでいないときは null。
  value: number | null
  onChange: (value: number) => void
  // 選べる範囲。GET /meta の rating(min・max)。取得できていない間は undefined で、入力を出さない(値を推測しない)。
  min: number | undefined
  max: number | undefined
}

// 評価の入力: 水位のバーガー + 選んだ数字 + 数字のボタン(ラジオボタンの集まり。押せるのは数字のボタンだけで、バーガーは見せるだけ)。
// ネイティブのラジオボタンなので、矢印キーで選ぶ値が変わり、フォーカスの移動もブラウザに任せる。
export function RatingInput({ name, label, value, onChange, min, max }: Props) {
  const { t } = useTranslation()
  if (min === undefined || max === undefined) return null
  const steps = Array.from({ length: max - min + 1 }, (_, i) => min + i)
  return (
    <fieldset className={styles.pick}>
      <legend className={styles.label}>{label}</legend>
      <div className={styles.box}>
        <div className={styles.view}>
          <RatingBurgerIcon ratio={value === null ? 0 : value / max} size="lg" />
          <div className={styles.num}>
            <b>{value ?? '–'}</b>
            <span className={styles.of}>{t('common.ratingOutOf', { max })}</span>
          </div>
        </div>
        <div className={styles.steps}>
          {steps.map((n) => (
            <label key={n} className={n === value ? `${styles.step} ${styles.on}` : styles.step}>
              <input
                type="radio"
                className={styles.radio}
                name={name}
                value={n}
                checked={n === value}
                onChange={() => onChange(n)}
              />
              {n}
            </label>
          ))}
        </div>
      </div>
    </fieldset>
  )
}
