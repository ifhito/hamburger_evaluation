import useSWR from "swr";
import { shopApiClient } from "../api/shopApiClient";
import type { Shop, ShopDetail } from "../api/types";
import { toPage, type Page } from "../../../api/page";
import { useInfinitePages } from "../../../api/useInfinitePages";

// per_page は送らない(1 ページの件数は backend が決める)
function buildKey(params: { keyword?: string } | undefined, page: number): string {
  const qs = new URLSearchParams();
  if (params?.keyword) qs.set("keyword", params.keyword);
  qs.set("page", String(page));
  return `/shops?${qs.toString()}`;
}

// 次のページがあるかは、前のページの hasMore(backend の X-Has-More)で決め、最終ページなら null を返す
export function getKey(params: { keyword?: string } | undefined) {
  return (index: number, previous: Page<Shop> | null): string | null =>
    previous && !previous.hasMore ? null : buildKey(params, index + 1);
}

export function useShops(params?: { keyword?: string }) {
  return useInfinitePages<Shop>(getKey(params), async (url: string) =>
    toPage(await shopApiClient.get<Shop[]>(url)),
  );
}

// viewerId は、API の返す内容(can_review。pending の shop の見え方も)が閲覧者ごとに違うため、キーに含める。
// ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュを再利用しない。
// キーの先頭が "/shops" なので、useShopMutations の isShopKey で破棄される。
// enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、閲覧者向けの応答が匿名キーに入ってしまう。
// enabled が false、または id が空なら null を返して取得しない。
export function shopDetailKey(id: number | undefined, viewerId: number | null, enabled = true) {
  return enabled && id ? (["/shops", id, viewerId] as const) : null;
}

export function useShopDetail(
  id: number | undefined,
  viewerId: number | null,
  options?: { enabled?: boolean },
) {
  return useSWR<ShopDetail>(shopDetailKey(id, viewerId, options?.enabled ?? true), async ([, shopId]) => {
    const res = await shopApiClient.get<ShopDetail>(`/shops/${shopId}`);
    return res.data;
  });
}
