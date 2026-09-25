import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerRanking } from "../api/types";
import { toPage, type Page } from "../../../api/page";
import { useInfinitePages } from "../../../api/useInfinitePages";

// per_page は送らない。検索と並び順を全ページのキーに含め、条件の異なる応答を混在させない。
export interface BurgerSearchParams { keyword?: string; sort?: string }
export function getKey(index: number, previous: Page<BurgerRanking> | null, params?: BurgerSearchParams): string | null {
  if (previous && !previous.hasMore) return null;
  const qs = new URLSearchParams();
  if (params?.keyword) qs.set("keyword", params.keyword);
  if (params?.sort) qs.set("sort", params.sort);
  qs.set("page", String(index + 1));
  return `/burgers?${qs.toString()}`;
}

export function useBurgers(params?: BurgerSearchParams) {
  return useInfinitePages<BurgerRanking>((index, previous) => getKey(index, previous, params), async (url: string) =>
    toPage(await burgerApiClient.get<BurgerRanking[]>(url)),
  );
}
