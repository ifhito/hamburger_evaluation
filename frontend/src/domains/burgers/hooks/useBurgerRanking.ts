import useSWR from "swr";
import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerRanking } from "../api/types";
import { toPage, type Page } from "../../../api/page";

// バーガーランキング(GET /burgers。weighted_score の高い順)。トップページの抜粋表示専用で、ページングはしない
// (1 ページ目だけを取得し、呼び出し側が上位数件を切り出す。このトップページの抜粋は、常に 1 ページ目だけを
// 見せる設計のため、ページングを持たない。全件は /burgers の一覧ページで見られる)。
// per_page は送らない(1 ページの件数は backend が決める。他の一覧 hook と同じ規則)。
export function useBurgerRanking() {
  // 一覧の useBurgers と同じキーを共有するため、キャッシュ内の形も Page に揃える。
  const result = useSWR<Page<BurgerRanking>>("/burgers?page=1", async (url: string) => {
    const res = await burgerApiClient.get<BurgerRanking[]>(url);
    return toPage(res);
  });
  return { ...result, data: result.data?.items };
}
