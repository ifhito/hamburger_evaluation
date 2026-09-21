// アプリへの戻り先を、ブラウザで開いてよいかを確かめる。backend は登録済みの戻り先だけを返すが、
// 開く先が javascript: などのスキームだと、ページの中で任意のコードが動いてしまうので、
// 通信のスキーム(http・https)以外は開かない(規則の判断ではなく、ページを開くときの安全のための確認)。
export function isNavigable(target: string): boolean {
  try {
    const { protocol } = new URL(target);
    return protocol === "https:" || protocol === "http:";
  } catch {
    return false;
  }
}
