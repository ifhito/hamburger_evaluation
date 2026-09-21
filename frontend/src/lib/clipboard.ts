// クリップボードにテキストを書き込み、書き込めたかどうかを返す。
// クリップボードの機能がない環境(暗号化されていない HTTP のページなど)や、書き込みを拒否されたとき(権限がない、
// ページにフォーカスがない)は、例外にせず false を返す。呼び出し側は、その場合に手動でコピーできる表示へ切り替える。
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
