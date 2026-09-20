import { useSWRConfig } from "swr";
import { userApiClient } from "../api/userApiClient";
import type { AuthUser } from "../../auth/types";
import type { UserUpdateInput } from "../api/types";

// useUser のキー(先頭が "/users")をすべて対象にする。
// 更新・削除の後は mutate(isUserKey, undefined) でこのキャッシュを空にする。編集ページには useUser がなく、
// 再検証だけではプロフィールへ戻ったときに古い username / email が一瞬出てしまうため。
// revalidate は既定のまま(false にしない)。false だと、直前の取得の重複排除の記録(dedupingInterval 内)が
// 残り、戻ったときのマウント時の取得がスキップされて、data が空のまま止まることがある。
// キーが配列の useUser だけが対象で、useReviews の "/reviews..." や "$inf$..." のキーには影響しない。
export function isUserKey(key: unknown): boolean {
  return Array.isArray(key) && key[0] === "/users";
}

export function useUpdateUser(id: number) {
  const { mutate } = useSWRConfig();
  return {
    // PUT のレスポンスは常に本人ビュー(email / admin あり)なので AuthUser を返す。
    update: async (data: UserUpdateInput): Promise<AuthUser> => {
      const res = await userApiClient.put<AuthUser>(`/users/${id}`, { user: data });
      await mutate(isUserKey, undefined);
      return res.data;
    },
  };
}

export function useDeleteUser() {
  const { mutate } = useSWRConfig();
  return {
    destroy: async (id: number): Promise<void> => {
      await userApiClient.delete(`/users/${id}`);
      await mutate(isUserKey, undefined);
    },
  };
}
