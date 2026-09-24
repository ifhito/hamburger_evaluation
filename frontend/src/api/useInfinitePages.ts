import { useMemo } from "react";
import useSWRInfinite from "swr/infinite";
import { mergePages, type Page } from "./page";

// getKey の結果に scope を重ねる。scope が undefined なら素のキーのまま(閲覧者を区別しない一覧、または匿名の一覧)。
// scope が違えば同じ url でも別々にキャッシュされ、scope が同じでも url が違えば別々にキャッシュされる。
export function withScope(
  key: string | null,
  scope: string | undefined,
): string | readonly [string, string] | null {
  return key === null || scope === undefined ? key : ([scope, key] as const);
}

const INFINITE_KEY_PREFIX = "$inf$"; // useSWRInfinite が内部で一覧の集計キーに付ける接頭辞。swr の非公開の実装詳細だが、
// SWR の mutate(フィルタ関数) がこの接頭辞のキーを常に除外する(下の既存コメント、および swr 内部の
// /^\$(inf|sub)\$/ の除外)ため、フィルタでは一覧を再検証できない。ここでは $inf$ のキーを直接なめて、
// キーごとに mutate する(キーだけを渡す mutate は、この関数の revalidateAll: true の設定に従って、読み込み
// 済みの全ページを再取得する。対応する一覧が現在マウントされていなくても、デデュープの印だけ消すので、
// 次にその一覧がマウントされたときの取得が確実に新しくなる)。
// ponytail: 対象を絞らず、このアプリの useSWRInfinite の利用先を全部洗い替える(4 種類だけなので許容)。
// 種類が増えて負荷が気になったら、swr/infinite の unstable_serialize(getKey) で対象を絞る。
export function revalidateInfiniteLists(
  cache: { keys(): IterableIterator<string> },
  mutate: (key: string) => Promise<unknown>,
): Promise<unknown[]> {
  const keys = [...cache.keys()].filter((key) => key.startsWith(INFINITE_KEY_PREFIX));
  return Promise.all(keys.map((key) => mutate(key)));
}

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
  const scopedKey = (index: number, previous: Page<T> | null) => withScope(getKey(index, previous), scope);
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
