import useSWR, { useSWRConfig } from "swr";
import { shopApiClient } from "../api/shopApiClient";
import type {
  AdminShop,
  Shop,
  ShopCreateInput,
  ShopStatus,
  ShopUpdateInput,
} from "../api/types";

// 一覧のキー(文字列 "/shops…"・"/admin/shops…")と、詳細のキー(["/shops", id, viewerId])の両方に一致する
export function isShopKey(key: unknown): boolean {
  return (
    (typeof key === "string" && (key.startsWith("/shops") || key.startsWith("/admin/shops"))) ||
    (Array.isArray(key) && key[0] === "/shops")
  );
}

export function useCreateShop() {
  const { mutate } = useSWRConfig();
  return {
    create: async (data: ShopCreateInput): Promise<Shop> => {
      const res = await shopApiClient.post<Shop>("/shops", { shop: data });
      await mutate(isShopKey);
      return res.data;
    },
  };
}

export function useAdminShops(status?: ShopStatus) {
  const key = status ? `/admin/shops?status=${status}` : "/admin/shops";
  return useSWR<AdminShop[]>(key, async (url: string) => {
    const res = await shopApiClient.get<AdminShop[]>(url);
    return res.data;
  });
}

export function useShopModeration() {
  const { mutate } = useSWRConfig();
  return {
    approve: async (id: number): Promise<AdminShop> => {
      const res = await shopApiClient.post<AdminShop>(`/admin/shops/${id}/approve`, {});
      await mutate(isShopKey);
      return res.data;
    },
    reject: async (id: number, moderationNote: string): Promise<AdminShop> => {
      const res = await shopApiClient.post<AdminShop>(`/admin/shops/${id}/reject`, {
        moderationNote,
      });
      await mutate(isShopKey);
      return res.data;
    },
  };
}

export function useUpdateShop(id: number) {
  const { mutate } = useSWRConfig();
  return {
    update: async (data: ShopUpdateInput): Promise<AdminShop> => {
      const res = await shopApiClient.put<AdminShop>(`/admin/shops/${id}`, { shop: data });
      await mutate(isShopKey);
      return res.data;
    },
  };
}
