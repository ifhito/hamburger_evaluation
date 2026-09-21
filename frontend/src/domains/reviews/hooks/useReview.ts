import useSWR from "swr";
import { reviewApiClient } from "../api/reviewApiClient";
import type { ReviewView } from "../api/types";

// viewerId は、API の返す内容(can_edit)が閲覧者ごとに違うため、キーに含める。
// ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュを再利用しない。
// キーの先頭が "/reviews" なので、useReviewMutations の isReviewKey で破棄・再検証される。
// enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、本人向けの応答(can_edit が true)が匿名キーに入ってしまう。
// enabled が false、または id が空なら null を返して取得しない。
export function reviewKey(id: number | undefined, viewerId: number | null, enabled = true) {
  return enabled && id ? (["/reviews", id, viewerId] as const) : null;
}

export function useReview(id: number | undefined, viewerId: number | null, options?: { enabled?: boolean }) {
  return useSWR<ReviewView>(reviewKey(id, viewerId, options?.enabled ?? true), async ([, reviewId]) => {
    const res = await reviewApiClient.get<ReviewView>(`/reviews/${reviewId}`);
    return res.data;
  });
}
