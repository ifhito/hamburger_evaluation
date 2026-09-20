import useSWR from "swr";
import { userApiClient } from "../api/userApiClient";
import type { User } from "../api/types";

// viewerId は、API の返す内容(本人にだけ email が付く)が閲覧者ごとに違うため、キーに含める。
// ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュ(email 入り)を再利用しない。
// キーの先頭が "/users" なので、useUserMutations の isUserKey で再検証される。
// options.enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、本人の email 入りの応答が匿名キーに入ってしまう。
export function useUser(id: number, viewerId: number | null, options?: { enabled?: boolean }) {
  // NaN(/users/abc)・小数・0 以下・安全でない整数の id では取得しない
  const enabled = Number.isSafeInteger(id) && id > 0 && (options?.enabled ?? true);
  return useSWR(enabled ? (["/users", id, viewerId] as const) : null, async ([, userId]) => {
    const res = await userApiClient.get<User>(`/users/${userId}`);
    return res.data;
  });
}
