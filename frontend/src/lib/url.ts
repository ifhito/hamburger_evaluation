// 外部または戻り先の URL をブラウザで開いてよいか(http: / https: スキームのみ許可し、javascript: などを防ぐ)。
export function isHttpUrl(target: string): boolean {
  try {
    const { protocol } = new URL(target);
    return protocol === "https:" || protocol === "http:";
  } catch {
    return false;
  }
}
