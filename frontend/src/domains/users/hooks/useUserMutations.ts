import { useSWRConfig } from "swr";
import { userApiClient } from "../api/userApiClient";
import type { AuthUser } from "../../auth/types";
import type { UserUpdateInput } from "../api/types";

// useUser のキー(先頭が "/users")をすべて再検証の対象にする
function isUserKey(key: unknown): boolean {
  return Array.isArray(key) && key[0] === "/users";
}

export function useUpdateUser(id: number) {
  const { mutate } = useSWRConfig();
  return {
    // PUT のレスポンスは常に本人ビュー(email / admin あり)なので AuthUser を返す。
    update: async (data: UserUpdateInput): Promise<AuthUser> => {
      const res = await userApiClient.put<AuthUser>(`/users/${id}`, { user: data });
      await mutate(isUserKey);
      return res.data;
    },
  };
}

export function useDeleteUser() {
  const { mutate } = useSWRConfig();
  return {
    destroy: async (id: number): Promise<void> => {
      await userApiClient.delete(`/users/${id}`);
      await mutate(isUserKey);
    },
  };
}
