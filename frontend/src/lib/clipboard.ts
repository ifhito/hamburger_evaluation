// クリップボードにテキストを書き込む。書き込めたら true を返す。
// クリップボード API が無い環境(HTTP の非セキュアなコンテキストなど)や、書き込みを拒否されたとき
// (権限・フォーカスなし)は false を返し、例外にはしない。呼び出し側は、手動でコピーできる表示に切り替える。
export async function copyToClipboard(
  text: string,
  clipboard: Pick<Clipboard, "writeText"> | undefined = typeof navigator === "undefined" ? undefined : navigator.clipboard,
): Promise<boolean> {
  if (!clipboard?.writeText) return false;
  try {
    await clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
