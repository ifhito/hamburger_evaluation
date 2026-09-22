import { useRef, type KeyboardEvent } from 'react'
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

// 評価の入力: 水位のバーガー + 選んだ数字 + 数字のボタン(デザイン(design/redesign/review-new.html)の
// button[role="radio"] による ARIA のカスタム radiogroup)。ネイティブの radio ボタンは使わない。矢印キーでの移動・選択と
// roving tabindex(選んでいる札だけ tabIndex=0。まだ選んでいなければ先頭)は、ARIA Authoring Practices の radiogroup
// パターンどおりここで実装する。Enter / Space での選択は button のネイティブな挙動に任せる。
export function RatingInput({ name, label, value, onChange, min, max }: Props) {
  const { t } = useTranslation()
  const buttons = useRef<Record<number, HTMLButtonElement | null>>({})
  if (min === undefined || max === undefined) return null
  const steps = Array.from({ length: max - min + 1 }, (_, i) => min + i)
  const labelId = `${name}-label`
  const active = value ?? min

  const select = (n: number, moveFocus: boolean) => {
    onChange(n)
    if (moveFocus) buttons.current[n]?.focus()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    const i = steps.indexOf(active)
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') select(steps[(i + 1) % steps.length], true)
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') select(steps[(i - 1 + steps.length) % steps.length], true)
    else if (e.key === 'Home') select(steps[0], true)
    else if (e.key === 'End') select(steps[steps.length - 1], true)
    else return
    e.preventDefault()
  }

  return (
    <div className={styles.pick}>
      <span id={labelId} className={styles.label}>
        {label}
      </span>
      <div className={styles.box} role="radiogroup" aria-labelledby={labelId}>
        <div className={styles.view}>
          <RatingBurgerIcon ratio={value === null ? 0 : value / max} size="lg" />
          <div className={styles.num}>
            <b>{value ?? '–'}</b>
            <span className={styles.of}>{t('common.ratingOutOf', { max })}</span>
          </div>
        </div>
        <div className={styles.steps}>
          {steps.map((n) => (
            <button
              key={n}
              type="button"
              ref={(el) => {
                buttons.current[n] = el
              }}
              role="radio"
              aria-checked={n === value}
              tabIndex={n === active ? 0 : -1}
              className={n === value ? `${styles.step} ${styles.on}` : styles.step}
              onClick={() => select(n, false)}
              onKeyDown={onKeyDown}
            >
              {n}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
