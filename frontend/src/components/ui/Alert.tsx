import styles from './alert.module.css'

interface Props {
  // 画面側の見出し(例「レビューを保存できませんでした」)。API が返した文言は、message にそのまま入れる。
  title?: string
  // API が返した文言。書き換えない。複数あるときは箇条書きにする。
  message: string | string[]
}

// エラーの表示: 赤の枠 + 左の赤い帯 + 「!」のアイコン + 文言。色だけに頼らない。読み上げには role="alert" で伝わる。
// デザイン(design/redesign/states.html ほか)は、この箱を <section> にしている(<div> ではない)。
export function Alert({ title, message }: Props) {
  const messages = Array.isArray(message) ? message : [message]
  return (
    <section role="alert" className={styles.alert}>
      <i className={styles.band} aria-hidden="true" />
      <span className={styles.mark} aria-hidden="true">
        !
      </span>
      <div className={styles.body}>
        {title && <b className={styles.title}>{title}</b>}
        {messages.length === 1 ? (
          <p className={styles.msg}>{messages[0]}</p>
        ) : (
          <ul className={styles.list}>
            {messages.map((m, i) => (
              <li key={i}>{m}</li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
