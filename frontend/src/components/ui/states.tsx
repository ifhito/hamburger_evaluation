import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { RatingBurgerIcon } from './RatingBurger'
import styles from './states.module.css'

interface Props {
  // 押すと次に進める操作(例「レビューを書く」の Button)。なければ出さない。
  action?: ReactNode
}

// 空・読み込み中・見つからないときの画面。文言は locale から出す(遊びの文言。API が返すエラーの文言とは別)。
// 絵は装飾で、読み上げない。

// まだ何もないとき: 空のバーガー + 促し。
export function EmptyState({ action }: Props) {
  const { t } = useTranslation()
  return (
    <section className={styles.panel}>
      <RatingBurgerIcon ratio={0} size="lg" />
      <h2 className={styles.title}>{t('common.states.empty.title')}</h2>
      <p className={styles.sub}>{t('common.states.empty.description')}</p>
      {action}
    </section>
  )
}

// 読み込み中: 焼き途中のバーガー(見た目だけの水位。評価ではない)+ 灰色の帯。
export const LOADING_LEVEL = 0.6
export function Loading() {
  const { t } = useTranslation()
  return (
    <section className={styles.panel} role="status">
      <RatingBurgerIcon ratio={LOADING_LEVEL} size="lg" />
      <h2 className={styles.title}>{t('common.states.loading.title')}</h2>
      <p className={styles.sub}>{t('common.states.loading.description')}</p>
      <div className={styles.skels} aria-hidden="true">
        <div className={styles.skel} />
        <div className={styles.skel} />
        <div className={styles.skel} />
      </div>
    </section>
  )
}

// ページが見つからないとき(404)。
export function NotFound({ action }: Props) {
  const { t } = useTranslation()
  return (
    <section className={styles.panel}>
      <span className={styles.code}>{t('common.states.notFound.code')}</span>
      <h2 className={styles.title}>{t('common.states.notFound.title')}</h2>
      <p className={styles.sub}>{t('common.states.notFound.description')}</p>
      {action}
    </section>
  )
}
