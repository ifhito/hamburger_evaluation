import useSWR from "swr";
import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerDetail } from "../api/types";

// useShopDetail・useReview と違い、viewerId をキーに入れない: GET /burgers/{id} の応答には、can_review・can_edit の
// ような、閲覧者ごとに違う値がないため、認証状態の復元を待たずに、id が分かった時点で取得してよい。
export function useBurger(id: string | undefined) {
  return useSWR<BurgerDetail>(id ? (["/burgers", id] as const) : null, async ([, burgerId]) => {
    const res = await burgerApiClient.get<BurgerDetail>(`/burgers/${burgerId}`);
    return res.data;
  });
}
