import { reviewApiClient } from "../api/reviewApiClient";
import type { ReviewSearchParams, ReviewView } from "../api/types";
import { toPage, type Page } from "../../../api/page";
import { useInfinitePages } from "../../../api/useInfinitePages";

// クエリ文字列は URL に直接書くため、キーはワイヤ上の名前(snake_case)にする。
// per_page は送らない(1 ページの件数は backend が決める)
function buildKey(params: ReviewSearchParams | undefined, page: number): string {
  const qs = new URLSearchParams();
  if (params?.rating !== undefined) qs.set("rating", String(params.rating));
  if (params?.keyword) qs.set("keyword", params.keyword);
  if (params?.userId !== undefined) qs.set("user_id", params.userId);
  if (params?.burgerId !== undefined) qs.set("burger_id", params.burgerId);
  qs.set("page", String(page));
  return `/reviews?${qs.toString()}`;
}

// useSWRInfinite に渡すキー関数を作る。次のページがあるかは、前のページの hasMore(backend の
// X-Has-More)で決め、最終ページなら null を返して読み込みを止める。
// enabled が false なら、どのページも null を返して取得を止める
export function getKey(params: ReviewSearchParams | undefined, enabled = true) {
  return (index: number, previous: Page<ReviewView> | null): string | null => {
    if (!enabled) return null;
    return previous && !previous.hasMore ? null : buildKey(params, index + 1);
  };
}

// GET /reviews の中身は閲覧者ごとに違う(各レビューの can_edit。作者本人だけ true)ため、useShops と同じ理由で、
// 一覧のキャッシュにも viewerId を含める(useInfinitePages の scope)。匿名(viewerId が null)は scope を
// 付けない(素の URL のままのキーになり、ログイン中の誰とも重ならない)。
// options.enabled: 呼び出し側が取得を止められる。例: user_id が不正なとき、そのまま呼ぶと 422 になり、
// user_id を外して呼ぶと全件が返る。未指定なら常に取得する
export function useReviews(
  params: ReviewSearchParams | undefined,
  viewerId: string | null,
  options?: { enabled?: boolean },
) {
  return useInfinitePages<ReviewView>(
    getKey(params, options?.enabled !== false),
    async (url: string) => toPage(await reviewApiClient.get<ReviewView[]>(url)),
    { scope: viewerId ?? undefined },
  );
}
