// rating の表示のための定数と整形。範囲(1〜5)の有効・無効の判断は backend の domain だけが持つ。
// ここは選択肢と星の描画のための値で、範囲外の値でも例外にならないようにするだけである。
export const RATING_MAX = 5;

/** rating を「★★★☆☆」の形に整形する。範囲外の値は 0〜RATING_MAX に丸める(例外にしない)。 */
export function formatRating(rating: number): string {
  const filled = Math.min(Math.max(Math.trunc(rating), 0), RATING_MAX);
  return "★".repeat(filled) + "☆".repeat(RATING_MAX - filled);
}
