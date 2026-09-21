import useSWR from "swr";
import { authApi } from "../api/authApiClient";

// 外部のサービス(Google など)との結び付きの一覧。利用者ごとに違うので、キーに viewerId を含める(ログアウトや
// 別の利用者でのログイン後に、前の利用者の一覧を再利用しない)。enabled が false(そのサービスを使えない設定)のときは、
// 取得しない。
export function identitiesKey(viewerId: string | null, enabled: boolean) {
  return enabled && viewerId ? (["/me/identities", viewerId] as const) : null;
}

export function useIdentities(viewerId: string | null, enabled: boolean) {
  const { data, error, isLoading, mutate } = useSWR(identitiesKey(viewerId, enabled), () => authApi.listIdentities());
  return { identities: data?.identities, error, isLoading, refresh: mutate };
}
