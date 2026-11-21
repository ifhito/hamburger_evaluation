// 各ドメインの SWR キャッシュキーを識別する述語。書き込み操作のあとに、関連する詳細・ランキング等のキャッシュも併せて更新するために共有する。

// レビューの一覧キー(文字列 "/reviews…")と詳細キー(["/reviews", id, viewerId])に一致する。
export function isReviewKey(key: unknown): boolean {
  return (
    (typeof key === "string" && key.startsWith("/reviews")) ||
    (Array.isArray(key) && key[0] === "/reviews")
  );
}

// ショップの一覧キー("/shops…"・"/admin/shops…")と詳細キー(["/shops", id, viewerId])に一致する。
export function isShopKey(key: unknown): boolean {
  return (
    (typeof key === "string" && (key.startsWith("/shops") || key.startsWith("/admin/shops"))) ||
    (Array.isArray(key) && key[0] === "/shops")
  );
}

// バーガーのランキング・一覧キー("/burgers…")と詳細キー(["/burgers", id, viewerId])に一致する。
export function isBurgerKey(key: unknown): boolean {
  return (
    (typeof key === "string" && key.startsWith("/burgers")) ||
    (Array.isArray(key) && key[0] === "/burgers")
  );
}

// ユーザー詳細のキー(["/users", id, viewerId])に一致する。
export function isUserKey(key: unknown): boolean {
  return Array.isArray(key) && key[0] === "/users";
}
