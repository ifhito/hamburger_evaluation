import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "../locale/en";
import ja from "../locale/ja";

// 対応する言語(3 つ目以降は S50 の非ゴール)。
export type SupportedLanguage = "en" | "ja";
const SUPPORTED_LANGUAGES: readonly SupportedLanguage[] = ["en", "ja"];
export const LANGUAGE_STORAGE_KEY = "burgerstack:lang";

function isSupportedLanguage(value: string | null | undefined): value is SupportedLanguage {
  return SUPPORTED_LANGUAGES.includes(value as SupportedLanguage);
}

// localStorage が使えない環境(プライベートブラウズ等)や、window が無い環境(node での単体テストなど)でも、
// 例外を投げずにブラウザの言語へフォールバックする(AC4)。
function readStoredLanguage(): SupportedLanguage | null {
  if (typeof window === "undefined") return null;
  try {
    const stored = window.localStorage.getItem(LANGUAGE_STORAGE_KEY);
    return isSupportedLanguage(stored) ? stored : null;
  } catch {
    return null;
  }
}

function storeLanguage(language: SupportedLanguage): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, language);
  } catch {
    // 保存できなくても、その場の切り替えは続ける(AC4)。
  }
}

// 既定はブラウザの言語(navigator.language など)から、対応する言語に丸める。対応外の言語は英語にする。
function detectBrowserLanguage(): SupportedLanguage {
  if (typeof navigator === "undefined") return "en";
  const candidates = navigator.languages && navigator.languages.length > 0 ? navigator.languages : [navigator.language];
  for (const candidate of candidates) {
    const primary = candidate?.split("-")[0]?.toLowerCase();
    if (isSupportedLanguage(primary)) return primary;
  }
  return "en";
}

const initialLanguage = readStoredLanguage() ?? detectBrowserLanguage();

void i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, ja: { translation: ja } },
  lng: initialLanguage,
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});

// <html lang> を選んだ言語に合わせる(AC6)。以降の切り替え(i18n.changeLanguage)でも、このイベントで追従する。
// document が無い環境(node での単体テストなど)では、何もしない。
if (typeof document !== "undefined") document.documentElement.lang = initialLanguage;
i18n.on("languageChanged", (language) => {
  const normalized: SupportedLanguage = language === "ja" ? "ja" : "en";
  if (typeof document !== "undefined") document.documentElement.lang = normalized;
  storeLanguage(normalized);
});

export default i18n;
