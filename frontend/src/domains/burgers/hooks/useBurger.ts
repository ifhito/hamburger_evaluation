import useSWR from "swr";
import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerDetail } from "../api/types";

// viewerId は、API の返す内容(shops の一覧)が閲覧者ごとに違うため、キーに含める。
// backend の Burgers.Get は、shops を domain.ShopVisibilityFor(viewer).CanView で絞り込む(審査待ち・却下のショップは
// admin・申請者本人にだけ見える)。ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュを再利用しない。
// enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、本人向けの応答が匿名キーに入ってしまう。
// enabled が false、または id が空なら null を返して取得しない。
export function burgerKey(id: string | undefined, viewerId: string | null, enabled = true) {
  return enabled && id ? (["/burgers", id, viewerId] as const) : null;
}

export function useBurger(id: string | undefined, viewerId: string | null, options?: { enabled?: boolean }) {
  return useSWR<BurgerDetail>(burgerKey(id, viewerId, options?.enabled ?? true), async ([, burgerId]) => {
    const res = await burgerApiClient.get<BurgerDetail>(`/burgers/${burgerId}`);
    return res.data;
  });
}
