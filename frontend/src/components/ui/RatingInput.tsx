import { useId, useRef, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { RatingBurgerIcon } from './RatingBurger'
import styles from './ratingInput.module.css'

interface Props {
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
export function RatingInput({ label, value, onChange, min, max }: Props) {
  const { t } = useTranslation()
  const labelId = useId()
  const buttons = useRef<Record<number, HTMLButtonElement | null>>({})
  if (min === undefined || max === undefined) return null
  const steps = Array.from({ length: max - min + 1 }, (_, i) => min + i)
  // roving tabindex の対象。value が(まだ選んでいない、または渡された min・max の外にある)ときは、先頭(min)にする。
  // 対象を必ず steps の中に収めないと、どのボタンも tabIndex=0 を持たず、キーボードで群に入れなくなる。
  const active = value !== null && steps.includes(value) ? value : min

  // クリックでも矢印キーでも、選んだ値へ実際に DOM のフォーカスを移す(ブラウザ既定のクリック時フォーカスに頼らない。
  // 一部のブラウザは button のクリックでフォーカスを移さず、roving tabindex の記録が実際のフォーカスとずれる)。
  // すでに選んでいる値を選び直したときは、ネイティブの radio と同じく onChange を呼ばない。
  const select = (n: number) => {
    if (n !== value) onChange(n)
    buttons.current[n]?.focus()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    const i = steps.indexOf(active)
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') select(steps[(i + 1) % steps.length])
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') select(steps[(i - 1 + steps.length) % steps.length])
    else if (e.key === 'Home') select(steps[0])
    else if (e.key === 'End') select(steps[steps.length - 1])
    else return
    e.preventDefault()
  }

  return (
    <div className={styles.pick}>
      <span id={labelId} className={styles.label}>
        {label}
      </span>
      <div className={styles.box} role="radiogroup" aria-labelledby={labelId} aria-orientation="horizontal">
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
              onClick={() => select(n)}
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
