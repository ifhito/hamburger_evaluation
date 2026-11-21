import { useSWRConfig } from "swr";
import { shopApiClient } from "../api/shopApiClient";
import { isBurgerKey, isReviewKey, isShopKey } from "../../../api/cacheKeys";
import { revalidateInfiniteLists, useInfinitePages } from "../../../api/useInfinitePages";
import { toPage, type Page } from "../../../api/page";
import type {
  AdminShop,
  Shop,
  ShopCreateInput,
  ShopStatus,
  ShopUpdateInput,
} from "../api/types";

export { isShopKey };

// 店舗名や公開・閉業状態が変わると、レビュー詳細やバーガー詳細・ランキングに含まれる店舗情報も変わるため併せて再検証する。
async function revalidateShopRelated(
  cache: { keys(): IterableIterator<string> },
  mutate: ReturnType<typeof useSWRConfig>["mutate"],
): Promise<void> {
  await Promise.all([
    mutate(isShopKey),
    mutate(isReviewKey),
    mutate(isBurgerKey),
    revalidateInfiniteLists(cache, mutate),
  ]);
}

export function useCreateShop() {
  const { cache, mutate } = useSWRConfig();
  return {
    create: async (data: ShopCreateInput): Promise<Shop> => {
      const res = await shopApiClient.post<Shop>("/shops", { shop: data });
      await revalidateShopRelated(cache, mutate);
      return res.data;
    },
  };
}

export function adminShopsKey(status: ShopStatus | undefined, enabled = true) {
  return (index: number, previous: Page<AdminShop> | null): string | null => {
    if (!enabled || (previous && !previous.hasMore)) return null;
    const qs = new URLSearchParams();
    if (status) qs.set("status", status);
    qs.set("page", String(index + 1));
    return `/admin/shops?${qs.toString()}`;
  };
}

// ページサイズはbackendに任せ、管理者が切り替わったときもキャッシュを共有しない。
export function useAdminShops(status: ShopStatus | undefined, viewerId: string | null) {
  return useInfinitePages<AdminShop>(
    adminShopsKey(status, viewerId !== null),
    async (url: string) => toPage(await shopApiClient.get<AdminShop[]>(url)),
    { scope: viewerId ?? undefined },
  );
}

export function useShopModeration() {
  const { cache, mutate } = useSWRConfig();
  return {
    approve: async (id: string): Promise<AdminShop> => {
      const res = await shopApiClient.post<AdminShop>(`/admin/shops/${id}/approve`, {});
      await revalidateShopRelated(cache, mutate);
      return res.data;
    },
    reject: async (id: string, moderationNote: string): Promise<AdminShop> => {
      const res = await shopApiClient.post<AdminShop>(`/admin/shops/${id}/reject`, {
        moderationNote,
      });
      await revalidateShopRelated(cache, mutate);
      return res.data;
    },
    close: async (id: string): Promise<AdminShop> => {
      const res = await shopApiClient.post<AdminShop>(`/admin/shops/${id}/close`, {});
      await revalidateShopRelated(cache, mutate);
      return res.data;
    },
    reopen: async (id: string): Promise<AdminShop> => {
      const res = await shopApiClient.post<AdminShop>(`/admin/shops/${id}/reopen`, {});
      await revalidateShopRelated(cache, mutate);
      return res.data;
    },
  };
}

export function useUpdateShop(id: string) {
  const { cache, mutate } = useSWRConfig();
  return {
    update: async (data: ShopUpdateInput): Promise<AdminShop> => {
      const res = await shopApiClient.put<AdminShop>(`/admin/shops/${id}`, { shop: data });
      await revalidateShopRelated(cache, mutate);
      return res.data;
    },
  };
}
