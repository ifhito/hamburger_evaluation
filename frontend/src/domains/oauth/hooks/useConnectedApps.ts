import { ApiError } from "../../../api/client/buildApiClient";
import { useInfinitePages } from "../../../api/useInfinitePages";
import type { Page } from "../../../api/page";
import { grantsUrl, oauthApi } from "../api/oauthApiClient";
import type { ConnectedApp } from "../api/types";

// 次のページがあるかは、前のページの hasMore(backend の X-Has-More)で決め、最終ページなら null を返す。
// viewerId が確定していない間(認証状態の復元前)は、どのページも null を返して取得しない。
export function getKey(viewerId: string | null) {
  return (index: number, previous: Page<ConnectedApp> | null): string | null => {
    if (viewerId === null) return null;
    return previous && !previous.hasMore ? null : grantsUrl(index + 1);
  };
}

// OAuth の認可サーバーが無効なとき、API は 404(未登録)を返す。それは「この機能は使えない」の意味で、
// エラーではなく、画面には何も出さない。
export function isFeatureDisabled(e: unknown): boolean {
  return e instanceof ApiError && e.status === 404;
}

// 許可したアプリの一覧(続きは fetchNextPage で読み込む)と、取り消し。一覧は利用者ごとに違うので、キャッシュの
// キーに viewerId を含める(ログアウトや別の利用者でのログイン後に、前の利用者の一覧を再利用しない)。
export function useConnectedApps(viewerId: string | null) {
  const { refresh, ...list } = useInfinitePages<ConnectedApp>(
    getKey(viewerId),
    (url) => oauthApi.listApps(url),
    { scope: viewerId ?? "", shouldRetryOnError: (e) => !isFeatureDisabled(e) },
  );
  return {
    ...list,
    isDisabled: isFeatureDisabled(list.error),
    // 取り消したあと、読み込み済みの全ページを取り直す(取り消した許可は、backend が返さなくなる)。
    revoke: async (id: string): Promise<void> => {
      await oauthApi.revokeApp(id);
      await refresh();
    },
  };
}
