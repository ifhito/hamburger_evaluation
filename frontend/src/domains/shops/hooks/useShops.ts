import useSWR from "swr";
import { shopApiClient } from "../api/shopApiClient";
import type { Shop, ShopDetail } from "../api/types";
import { toPage, type Page } from "../../../api/page";
import { useInfinitePages } from "../../../api/useInfinitePages";

// per_page は送らない(1 ページの件数は backend が決める)
function buildKey(params: { keyword?: string; sort?: "newest" } | undefined, page: number): string {
  const qs = new URLSearchParams();
  if (params?.keyword) qs.set("keyword", params.keyword);
  if (params?.sort) qs.set("sort", params.sort);
  qs.set("page", String(page));
  return `/shops?${qs.toString()}`;
}

// 次のページがあるかは、前のページの hasMore(backend の X-Has-More)で決め、最終ページなら null を返す。
// enabled が false なら、どのページも null を返して取得を止める(認証状態の復元前に使う)
export function getKey(params: { keyword?: string; sort?: "newest" } | undefined, enabled = true) {
  return (index: number, previous: Page<Shop> | null): string | null => {
    if (!enabled) return null;
    return previous && !previous.hasMore ? null : buildKey(params, index + 1);
  };
}

// GET /shops の中身は閲覧者ごとに違う(domain.ShopVisibilityFor。管理者・申請者本人には pending・rejected も
// 見える)ため、useShopDetail と同じ理由で、一覧のキャッシュにも viewerId を含める(useInfinitePages の scope)。
// 匿名(viewerId が null)は scope を付けない(素の URL のままのキーになり、ログイン中の誰とも重ならない)。
// enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。token は localStorage にある
// のに viewerId が null のまま取得すると、閲覧者向けの応答が匿名のキーに入ってしまう。
export function useShops(
  params: { keyword?: string; sort?: "newest" } | undefined,
  viewerId: string | null,
  options?: { enabled?: boolean },
) {
  return useInfinitePages<Shop>(
    getKey(params, options?.enabled ?? true),
    async (url: string) => toPage(await shopApiClient.get<Shop[]>(url)),
    { scope: viewerId ?? undefined },
  );
}

// viewerId は、API の返す内容(can_review。pending の shop の見え方も)が閲覧者ごとに違うため、キーに含める。
// ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュを再利用しない。
// キーの先頭が "/shops" なので、useShopMutations の isShopKey で破棄される。
// enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、閲覧者向けの応答が匿名キーに入ってしまう。
// enabled が false、または id が空なら null を返して取得しない。
export function shopDetailKey(id: string | undefined, viewerId: string | null, enabled = true) {
  return enabled && id ? (["/shops", id, viewerId] as const) : null;
}

export function useShopDetail(
  id: string | undefined,
  viewerId: string | null,
  options?: { enabled?: boolean },
) {
  return useSWR<ShopDetail>(shopDetailKey(id, viewerId, options?.enabled ?? true), async ([, shopId]) => {
    const res = await shopApiClient.get<ShopDetail>(`/shops/${shopId}`);
    return res.data;
  });
}
