import useSWR, { useSWRConfig } from "swr";
import { ApiError } from "../../../api/client/buildApiClient";
import { oauthApi } from "../api/oauthApiClient";

// viewerId は、許可したアプリが利用者ごとに違うため、キーに含める(ログアウトや別の利用者でのログイン後に、
// 前の利用者の一覧を再利用しない)。viewerId が確定していないときは null にして、取得しない。
export function connectedAppsKey(viewerId: string | null) {
  return viewerId ? (["/oauth/grants", viewerId] as const) : null;
}

// OAuth の認可サーバーが無効なとき、API は 404(未登録)を返す。それは「この機能は使えない」の意味で、
// エラーではなく、画面には何も出さない。
export function isFeatureDisabled(e: unknown): boolean {
  return e instanceof ApiError && e.status === 404;
}

export function useConnectedApps(viewerId: string | null) {
  const { data, error, isLoading } = useSWR(connectedAppsKey(viewerId), () => oauthApi.listApps(), {
    shouldRetryOnError: (e) => !isFeatureDisabled(e),
  });
  return { data, error, isLoading, isDisabled: isFeatureDisabled(error) };
}

export function useRevokeConnectedApp() {
  const { mutate } = useSWRConfig();
  return {
    // 取り消したあと、一覧を取り直す(取り消した許可は、backend が返さなくなる)。
    revoke: async (id: string): Promise<void> => {
      await oauthApi.revokeApp(id);
      await mutate((key) => Array.isArray(key) && key[0] === "/oauth/grants");
    },
  };
}
