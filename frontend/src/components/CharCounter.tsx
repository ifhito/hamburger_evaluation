import { useTranslation } from 'react-i18next'
import { countChars } from '../lib/countChars'
import styles from './field.module.css'

interface CharCounterProps {
  id: string
  value: string
  // GET /meta の上限。取得できていない間は undefined で、カウンターを出さない(値を推測しない)。
  max: number | undefined
}

// 入力欄の文字数を「いまの文字数 / 上限」の形で見せる、表示だけの部品。上限を超えても、入力も送信も止めない
// (判定は backend の 422 に任せる)。超えている間は、色だけでなく「Too long」の文字でも知らせる。
// 読み上げには、数字が変わるたびではなく、上限を超えたときと戻ったときの 2 回だけ伝わるよう、
// 見えない role="status" の文言を、超えた/収まっている の 2 つだけにしている。
export function CharCounter({ id, value, max }: CharCounterProps) {
  const { t } = useTranslation()
  if (max === undefined) return null
  const count = countChars(value)
  const over = count > max
  return (
    <div className={styles.counterRow}>
      <span id={id} className={over ? styles.counterOver : styles.counter}>
        {count} / {max}
        {over && ` ${t('common.charCounter.tooLong')}`}
      </span>
      <span role="status" className={styles.srOnly}>
        {over ? t('common.charCounter.overAnnounce', { max }) : t('common.charCounter.withinAnnounce')}
      </span>
    </div>
  )
}
