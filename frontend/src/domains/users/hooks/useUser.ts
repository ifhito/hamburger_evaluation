import useSWR from "swr";
import { ApiError } from "../../../api/client/buildApiClient";
import { userApiClient } from "../api/userApiClient";
import type { User } from "../api/types";

// viewerId は、API の返す内容(本人にだけ email が付く)が閲覧者ごとに違うため、キーに含める。
// ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュ(email 入り)を再利用しない。
// キーの先頭が "/users" なので、useUserMutations の isUserKey で破棄される。
// enabled: viewerId がまだ確定していない間(認証状態の復元前)は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、本人の email 入りの応答が匿名キーに入ってしまう。
// enabled が false、または id がないなら null を返して取得しない。
// id の形式の判定は持たない。正規形でない id は API が 404 を返し、それをエラー表示に使う。
export function userKey(id: string | undefined, viewerId: string | null, enabled = true) {
  return enabled && id ? (["/users", id, viewerId] as const) : null;
}

function isNotFound(e: unknown): boolean {
  return e instanceof ApiError && e.status === 404;
}

export function useUser(id: string | undefined, viewerId: string | null, options?: { enabled?: boolean }) {
  const { data, error, isLoading } = useSWR(
    userKey(id, viewerId, options?.enabled ?? true),
    async ([, userId]) => {
      const res = await userApiClient.get<User>(`/users/${userId}`);
      return res.data;
    },
    // 退会済みなどの 404 は再取得しても変わらないので、指数バックオフの再取得を続けない
    { shouldRetryOnError: (e) => !isNotFound(e) },
  );
  // SWR はエラーになっても取得済みの data を保持する。退会して 404 になったユーザーの古いプロフィールを返さない
  return { data: isNotFound(error) ? undefined : data, error, isLoading };
}
