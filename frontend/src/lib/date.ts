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

// 実食日(日付のみの "YYYY-MM-DD")を表示用に整形する。"T00:00:00" を足して、ブラウザのローカル
// タイムゾーンとして解釈させる(素の "YYYY-MM-DD" を Date にすると UTC として解釈され、UTC より
// 遅れたタイムゾーンで前日にずれるため)。
export function formatVisitedAt(dateOnly: string): string {
  const date = new Date(`${dateOnly}T00:00:00`);
  const locale = DATE_LOCALE[i18n.language] ?? DATE_LOCALE.en;
  return date.toLocaleDateString(locale, { year: "numeric", month: "short", day: "numeric" });
}

// <input type="date"> の max に使う、今日の日付(ローカルのタイムゾーン)を "YYYY-MM-DD" で返す。
export function todayDateOnly(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}
