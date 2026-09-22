import { useTranslation } from 'react-i18next'
import type { SupportedLanguage } from '../lib/i18n'
import styles from './languageSwitcher.module.css'

// ヘッダーの JA / EN 切り替え(design/redesign/mock.css の .lang / .lang-btn と同じ見た目・要素(button))。
// 選択の保存・<html lang> の更新は lib/i18n.ts(languageChanged のイベント)が行う。
const LANGUAGES: SupportedLanguage[] = ['ja', 'en']
const LABELS: Record<SupportedLanguage, string> = { ja: 'JA', en: 'EN' }

export function LanguageSwitcher({ className }: { className?: string }) {
  const { i18n } = useTranslation()
  const current: SupportedLanguage = i18n.language === 'ja' ? 'ja' : 'en'

  return (
    <div className={className ? `${styles.lang} ${className}` : styles.lang}>
      {LANGUAGES.map((lang) => (
        <button
          key={lang}
          type="button"
          className={lang === current ? `${styles.langBtn} ${styles.on}` : styles.langBtn}
          aria-pressed={lang === current}
          onClick={() => void i18n.changeLanguage(lang)}
        >
          {LABELS[lang]}
        </button>
      ))}
    </div>
  )
}
