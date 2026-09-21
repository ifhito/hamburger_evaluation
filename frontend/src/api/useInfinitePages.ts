import { useMemo } from "react";
import useSWRInfinite from "swr/infinite";
import { mergePages, type Page } from "./page";

// 続きを読み込める一覧(GET /shops・GET /reviews)の共通の取得フック。
// 次のページがあるかは、各ページの hasMore(backend が X-Has-More で返す)だけで決める。
// getKey は、前のページの hasMore が false なら null を返して読み込みを止める。
// options.scope: 応答が利用者ごとに違う一覧(例: 自分が許可したアプリ)では、利用者の id を渡す。キャッシュのキーに含まれ、
// ログアウトや別の利用者でのログイン後に、前の利用者の一覧を再利用しない(fetchPage には、これを含まない URL を渡す)。
// options.shouldRetryOnError: 失敗したときに、間をおいて取り直すかの判断(既定は取り直す)。
export function useInfinitePages<T extends { id: number | string }>(
  getKey: (index: number, previous: Page<T> | null) => string | null,
  fetchPage: (url: string) => Promise<Page<T>>,
  options?: { scope?: string; shouldRetryOnError?: (error: unknown) => boolean },
) {
  const scope = options?.scope;
  const scopedKey = (index: number, previous: Page<T> | null) => {
    const url = getKey(index, previous);
    return url === null || scope === undefined ? url : ([scope, url] as const);
  };
  const { data, error, isLoading, size, setSize, mutate } = useSWRInfinite<Page<T>>(
    scopedKey,
    (key: string | readonly [string, string]) => fetchPage(typeof key === "string" ? key : key[1]),
    // 読み込み済みの全ページを再検証する。SWR の mutate(フィルタ関数)は `$inf$` で始まる無限ロードの
    // キーを飛ばす(swr 内部で /^\$(inf|sub)\$/ に一致するキーは除外される)ため、書き込み後の更新は
    // 「一覧に戻ったときのマウント時の再検証」に頼る。既定の revalidateFirstPage だけだと先頭ページしか
    // 再取得されず、新規投稿で押し出された 1 件が 2 ページ目のキャッシュから抜ける。それを防ぐために true にしている。
    // 代償として、Load more のたびだけでなく、フォーカス復帰(5 秒スロットル)・再接続・エラーリトライでも、
    // 読み込み済みの全ページを直列に再取得する(リクエスト数はページ数に比例する)
    { revalidateAll: true, ...(options?.shouldRetryOnError ? { shouldRetryOnError: options.shouldRetryOnError } : {}) },
  );

  const items = useMemo(() => (data ? mergePages(data) : undefined), [data]);

  return {
    data: items,
    error,
    isLoading,
    hasNextPage: data !== undefined && data.length > 0 && data[data.length - 1].hasMore,
    // 要求したページがまだ届いていない間
    isFetchingNextPage: !error && size > 1 && data !== undefined && data[size - 1] === undefined,
    // 読み込み済みの全ページを取り直す(書き込みのあとに、一覧を最新にする)
    refresh: () => mutate(),
    fetchNextPage: () => {
      // size は失敗時も n+1 のままなので (s) => s + 1 だと再クリックで n+2 に飛ぶ。読み込み済みページ数から数える
      void setSize((data?.length ?? 0) + 1);
    },
  };
}
