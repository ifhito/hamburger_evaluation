import { useTranslation } from 'react-i18next'
import type { SupportedLanguage } from '../lib/i18n'
import styles from './languageSwitcher.module.css'

// 言語切り替え。フッターでは言語名を省略せず表示する。
// 選択の保存・<html lang> の更新は lib/i18n.ts(languageChanged のイベント)が行う。
const LANGUAGES: SupportedLanguage[] = ['ja', 'en']
const LABELS: Record<SupportedLanguage, string> = { ja: 'JA', en: 'EN' }

export function LanguageSwitcher({ className, fullLabels = false }: { className?: string; fullLabels?: boolean }) {
  const { i18n } = useTranslation()
  const current: SupportedLanguage = i18n.language === 'ja' ? 'ja' : 'en'

  return (
    <div className={[styles.lang, fullLabels && styles.full, className].filter(Boolean).join(" ")}>
      {LANGUAGES.map((lang) => (
        <button
          key={lang}
          type="button"
          className={lang === current ? `${styles.langBtn} ${styles.on}` : styles.langBtn}
          aria-pressed={lang === current}
          onClick={() => void i18n.changeLanguage(lang)}
        >
          {fullLabels ? (lang === "ja" ? "日本語" : "English") : LABELS[lang]}
        </button>
      ))}
    </div>
  )
}
