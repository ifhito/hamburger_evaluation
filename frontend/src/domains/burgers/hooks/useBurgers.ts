import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerRanking } from "../api/types";
import { toPage, type Page } from "../../../api/page";
import { useInfinitePages } from "../../../api/useInfinitePages";

// per_page は送らない(1 ページの件数は backend が決める)。weighted_score の高い順は backend が返す既定の順で、
// frontend からは並び替え・絞り込みのパラメータを送らない(#196 の非ゴール)。
export function getKey(index: number, previous: Page<BurgerRanking> | null): string | null {
  return previous && !previous.hasMore ? null : `/burgers?page=${index + 1}`;
}

export function useBurgers() {
  return useInfinitePages<BurgerRanking>(getKey, async (url: string) =>
    toPage(await burgerApiClient.get<BurgerRanking[]>(url)),
  );
}
