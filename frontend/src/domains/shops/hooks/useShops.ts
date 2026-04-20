import useSWR from "swr";
import { shopApiClient } from "../api/shopApiClient";
import type { Shop, ShopDetail } from "../api/types";

export function useShops(params?: { keyword?: string }) {
  const key = params?.keyword
    ? `/shops?keyword=${encodeURIComponent(params.keyword)}`
    : "/shops";

  return useSWR<Shop[]>(key, async (url: string) => {
    const res = await shopApiClient.get<Shop[]>(url);
    if (!Array.isArray(res.data)) {
      throw new Error("Invalid response: expected array");
    }
    return res.data;
  });
}

export function useShopDetail(id: number | undefined) {
  return useSWR<ShopDetail>(
    id ? `/shops/${id}` : null,
    async (url: string) => {
      const res = await shopApiClient.get<ShopDetail>(url);
      return res.data;
    }
  );
}
