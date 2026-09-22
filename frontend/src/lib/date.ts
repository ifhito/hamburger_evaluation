// UI の文言は英語(lib/i18n.ts の lng: "en"。日本語は S50)なので、日付も英語の書式にする
// (デザイン(design/redesign)の "Sep 21, 2026" と同じ、月を短縮した書式)。
export function formatDate(isoString: string): string {
  const date = new Date(isoString);
  return date.toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
