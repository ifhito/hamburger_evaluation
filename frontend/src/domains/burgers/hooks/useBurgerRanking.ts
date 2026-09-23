import useSWR from "swr";
import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerRanking } from "../api/types";
import { toPage } from "../../../api/page";

// バーガーランキング(GET /burgers。weighted_score の高い順)。トップページの抜粋表示専用で、ページングはしない
// (1 ページ目だけを取得し、呼び出し側が上位数件を切り出す。「もっと見る」導線は、専用の一覧ページがまだ無いため置かない)。
// per_page は送らない(1 ページの件数は backend が決める。他の一覧 hook と同じ規則)。
export function useBurgerRanking() {
  return useSWR<BurgerRanking[]>("/burgers?page=1", async (url: string) => {
    const res = await burgerApiClient.get<BurgerRanking[]>(url);
    return toPage(res).items;
  });
}
