// rating を「★★★☆☆」の形に整形する(表示だけ)。範囲(最小・最大)の値と、有効・無効の判断は
// backend の domain だけが持ち、GET /meta で受け取った max を渡す。max が分かるまで(undefined)は、
// 空の星を描かず、rating の分の ★ だけを返す。範囲外の値でも例外にしない。
export function formatRating(rating: number, max: number | undefined): string {
  const filled = Math.max(Math.trunc(rating), 0);
  if (max === undefined) return "★".repeat(filled);
  const shown = Math.min(filled, max);
  return "★".repeat(shown) + "☆".repeat(max - shown);
}
