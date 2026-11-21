import { useSWRConfig } from "swr";
import { userApiClient } from "../api/userApiClient";
import { isReviewKey, isShopKey, isUserKey } from "../../../api/cacheKeys";
import { revalidateInfiniteLists } from "../../../api/useInfinitePages";
import type { AuthUser } from "../../auth/types";
import type { UserUpdateInput } from "../api/types";

export { isUserKey };

// ユーザー名変更ではプロフィール本体と関連ドメインを再検証し、退会では削除済みユーザーの再取得(404)が走らないよう再検証なしで消去する。
export function useUpdateUser(id: string) {
  const { cache, mutate } = useSWRConfig();
  return {
    // PUT のレスポンスは常に本人ビューなので AuthUser を返す。
    update: async (data: UserUpdateInput): Promise<AuthUser> => {
      const res = await userApiClient.put<AuthUser>(`/users/${id}`, { user: data });
      await Promise.all([
        mutate(isUserKey),
        mutate(isReviewKey),
        mutate(isShopKey),
        revalidateInfiniteLists(cache, mutate),
      ]);
      return res.data;
    },
  };
}

export function useDeleteUser() {
  const { mutate } = useSWRConfig();
  return {
    destroy: async (id: string): Promise<void> => {
      await userApiClient.delete(`/users/${id}`);
      await mutate(isUserKey, undefined, { revalidate: false });
    },
  };
}
