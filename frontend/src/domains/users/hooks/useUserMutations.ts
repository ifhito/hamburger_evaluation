import { useSWRConfig } from "swr";
import { userApiClient } from "../api/userApiClient";
import type { AuthUser } from "../../auth/types";
import type { UserUpdateInput } from "../api/types";

// useUser が使うキャッシュのキー(["/users", id, viewerId])だけに一致する。レビュー一覧のキーには一致しない。
// 更新・削除の後は、一致したキャッシュを mutate(isUserKey, undefined) で空にする。
//   なぜ空にするか: 編集ページにはプロフィールを取得する useUser がないので、再取得だけでは足りない。
//   空にしないと、プロフィールへ戻ったときに、古い名前やメールが一瞬表示されてしまう。
//   なぜ { revalidate: false } を付けないか: 付けると、戻ったときの取得が省略されて、何も表示されないまま止まることがある。
export function isUserKey(key: unknown): boolean {
  return Array.isArray(key) && key[0] === "/users";
}

export function useUpdateUser(id: string) {
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
    destroy: async (id: string): Promise<void> => {
      await userApiClient.delete(`/users/${id}`);
      await mutate(isUserKey, undefined);
    },
  };
}
