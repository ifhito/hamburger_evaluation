import type { AxiosResponse } from "axios";

// 一覧 API(GET /shops・GET /reviews)の 1 ページぶん。hasMore は次のページがあるかで、backend が
// レスポンスヘッダー X-Has-More で返す。frontend は 1 ページの件数を知らず、件数から最終ページを推測しない。
export interface Page<T> {
  items: T[];
  hasMore: boolean;
}

// 一覧のレスポンス(配列 + X-Has-More)を Page に変える。形が想定と違うときは黙って続けず失敗させる
// (ヘッダーが proxy に落とされると、続きが読めなくなるのを気づけないため)。
export function toPage<T>(res: AxiosResponse<T[]>): Page<T> {
  if (!Array.isArray(res.data)) {
    throw new Error("Invalid response: expected array");
  }
  const hasMore = res.headers["x-has-more"];
  if (hasMore !== "true" && hasMore !== "false") {
    throw new Error("Invalid response: missing X-Has-More header");
  }
  return { items: res.data, hasMore: hasMore === "true" };
}

// offset ページングでは、ページの間に新規投稿があると前ページ末尾が次ページに再登場するため、
// id で重複を除く(先頭出現の位置を保つ)。削除で 1 件飛ぶ場合は解消できない既知の制約
export function mergePages<T extends { id: number | string }>(pages: Page<T>[]): T[] {
  return [...new Map(pages.flatMap((p) => p.items).map((item) => [item.id, item])).values()];
}
