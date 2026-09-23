import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { resolveBurger, roundRating, type BurgerSize, type BurgerVariant } from './burgerShapes'
import styles from './ratingBurger.module.css'

interface IconProps {
  // 塗る高さの割合(0〜1。範囲外は、0 以下は空・1 以上は満タンと同じ)。評価の値と最大値の比。
  ratio: number
  size?: BurgerSize
  // 'level'(水位。既定): 小数の違いが見た目に出る。平均評価に使う。'stepped'(部品ごとの段階): 段階がはっきり
  // 分かる代わりに小数を区別できない。1 件の評価・評価の入力(必ず整数)に使う。
  variant?: BurgerVariant
}

// バーガーの絵だけ(装飾。読み上げの対象にしない)。数字を伴う表示は RatingBurger を使う。数字を大きく見せるなど、
// 数字の見せ方を変える画面は、この絵と、自分の数字を組み合わせる。
export function RatingBurgerIcon({ ratio, size = 'md', variant = 'level' }: IconProps) {
  const icon = resolveBurger(ratio, size, variant)
  return (
    <svg
      className={styles.burger}
      viewBox={`0 0 ${icon.vb[0]} ${icon.vb[1]}`}
      width={icon.width}
      height={icon.height}
      aria-hidden="true"
      focusable="false"
    >
      {icon.shapes.map((s, i) => (
        <path
          key={i}
          d={s.d}
          fill="none"
          stroke={s.stroke}
          strokeWidth={s.sw}
          strokeLinejoin="round"
          strokeLinecap="round"
          strokeDasharray={s.dash ?? undefined}
        />
      ))}
    </svg>
  )
}

interface Props {
  value: number
  // 評価の最大値。GET /meta の rating.max(frontend に定数を持たない)。
  max: number
  size?: BurgerSize
  // 数字の小数の桁数(平均を「4.0」と見せるときなど)。指定しなければ、小数 1 桁に丸めた値をそのまま(4 → 「4」、4.5 → 「4.5」)。
  fractionDigits?: number
  // 1 件の評価(必ず整数)は 'stepped'。平均評価(小数になる)は既定の 'level' のまま。
  variant?: BurgerVariant
}

// 評価のバーガー + 数字。数字を必ずそばに書く(形や色だけで伝えない)。水位・段階も数字と同じ、小数 1 桁に丸めた値で決める。
// 読み上げは「Rating 4.5 out of 5」の 1 つの画像として伝え、絵と数字を二重に読み上げない。
export function RatingBurger({ value, max, size = 'md', fractionDigits, variant }: Props) {
  const { t } = useTranslation()
  const shown = roundRating(value)
  const text = fractionDigits === undefined ? String(shown) : shown.toFixed(fractionDigits)
  return (
    <span role="img" aria-label={t('common.ratingAria', { value: text, max })} className={styles.rate}>
      <RatingBurgerIcon ratio={max > 0 ? shown / max : 0} size={size} variant={variant} />
      <b aria-hidden="true">{text}</b>
    </span>
  )
}

interface SummaryProps {
  // 平均。レビューがまだなければ null(デザインの大きな数字は出さない)。
  value: number | null
  // GET /meta の rating.max。未取得なら undefined で、装飾のバーガーは空のまま。
  max: number | undefined
  count: number
  // 「レビューを書く」などの操作(あれば、右端に置く)。
  action?: ReactNode
}

// ショップ詳細で使う、大きな平均値 + 装飾のバーガー + 件数(design/redesign の .summary /
// .shop-summary)。RatingBurger と違い、数字とバーガーを別々の大きさで見せるための専用の組み合わせ。
export function RatingSummary({ value, max, count, action }: SummaryProps) {
  const { t } = useTranslation()
  const ratio = value !== null && max !== undefined && max > 0 ? roundRating(value) / max : 0
  return (
    <section className={styles.summary}>
      {value !== null && (
        <span
          className={styles.avg}
          aria-label={max !== undefined ? t('common.ratingAria', { value: value.toFixed(1), max }) : undefined}
        >
          {value.toFixed(1)}
        </span>
      )}
      <div className={styles.summaryCount}>
        <RatingBurgerIcon ratio={ratio} size="lg" />
        <p className={styles.count}>{t('common.reviewCount', { count })}</p>
      </div>
      {action && <div className={styles.summaryAction}>{action}</div>}
    </section>
  )
}
