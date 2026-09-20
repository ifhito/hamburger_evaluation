import { useMemo } from "react";
import useSWRInfinite from "swr/infinite";
import { reviewApiClient } from "../api/reviewApiClient";
import type { Review, ReviewSearchParams } from "../api/types";

// 「返却件数が PER_PAGE 未満なら最終ページ」の判定と、API へ送る per_page を一致させる
export const PER_PAGE = 20;

// クエリ文字列は URL に直接書くため、キーはワイヤ上の名前(snake_case)にする
function buildKey(params: ReviewSearchParams | undefined, page: number): string {
  const qs = new URLSearchParams();
  if (params?.rating !== undefined) qs.set("rating", String(params.rating));
  if (params?.keyword) qs.set("keyword", params.keyword);
  if (params?.userId !== undefined) qs.set("user_id", String(params.userId));
  qs.set("page", String(page));
  qs.set("per_page", String(PER_PAGE));
  return `/reviews?${qs.toString()}`;
}

// offset ページングでは、ページの間に新規投稿があると前ページ末尾が次ページに再登場するため、
// id で重複を除く(先頭出現の位置を保つ)。削除で 1 件飛ぶ場合は解消できない既知の制約
export function mergePages(pages: Review[][]): Review[] {
  return [...new Map(pages.flat().map((r) => [r.id, r])).values()];
}

// 直近のページが PER_PAGE 件そろっているときだけ、次のページがある
export function hasNextPage(pages: Review[][] | undefined): boolean {
  const last = pages?.[pages.length - 1];
  return last !== undefined && last.length >= PER_PAGE;
}

// useSWRInfinite に渡すキー関数を作る。前ページが最終ページなら null を返して読み込みを止める。
// 次ページ判定は hasNextPage に一本化している
export function getKey(params: ReviewSearchParams | undefined) {
  return (index: number, previous: Review[] | null): string | null =>
    previous && !hasNextPage([previous]) ? null : buildKey(params, index + 1);
}

export function useReviews(params?: ReviewSearchParams) {
  const { data, error, isLoading, size, setSize } = useSWRInfinite<Review[]>(
    getKey(params),
    async (url: string) => {
      const res = await reviewApiClient.get<Review[]>(url);
      if (!Array.isArray(res.data)) {
        throw new Error("Invalid response: expected array");
      }
      return res.data;
    },
    // 読み込み済みの全ページを再検証する。SWR の mutate(フィルタ関数)は `$inf$` で始まる無限ロードの
    // キーを飛ばす(swr 内部で /^\$(inf|sub)\$/ に一致するキーは除外される)ため、書き込み後の更新は
    // 「一覧に戻ったときのマウント時の再検証」に頼る。既定の revalidateFirstPage だけだと先頭ページしか
    // 再取得されず、新規投稿で押し出された 1 件が 2 ページ目のキャッシュから抜ける。それを防ぐために true にしている。
    // 代償として、Load more のたびだけでなく、フォーカス復帰(5 秒スロットル)・再接続・エラーリトライでも、
    // 読み込み済みの全ページを直列に再取得する(リクエスト数はページ数に比例する)
    { revalidateAll: true }
  );

  const reviews = useMemo(() => (data ? mergePages(data) : undefined), [data]);

  return {
    data: reviews,
    error,
    isLoading,
    hasNextPage: hasNextPage(data),
    // 要求したページがまだ届いていない間
    isFetchingNextPage: !error && size > 1 && data !== undefined && data[size - 1] === undefined,
    fetchNextPage: () => {
      // size は失敗時も n+1 のままなので (s) => s + 1 だと再クリックで n+2 に飛ぶ。読み込み済みページ数から数える
      void setSize((data?.length ?? 0) + 1);
    },
  };
}
