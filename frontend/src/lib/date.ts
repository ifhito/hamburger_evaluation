import i18n from "./i18n";

// 選んだ言語(lib/i18n.ts)に合わせて、日付の書式を切り替える(R3)。デザイン(design/redesign)の
// 英語は "Sep 21, 2026"(月を短縮)、日本語は "2026年9月21日"。
const DATE_LOCALE: Record<string, string> = { ja: "ja-JP", en: "en-US" };

export function formatDate(isoString: string): string {
  const date = new Date(isoString);
  const locale = DATE_LOCALE[i18n.language] ?? DATE_LOCALE.en;
  return date.toLocaleDateString(locale, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
