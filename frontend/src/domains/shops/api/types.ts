import type { Review } from "../../reviews/api/types";

export type ShopStatus = "pending" | "active" | "rejected";

export interface Shop {
  id: number;
  name: string;
  status: ShopStatus;
}

export interface ShopCreator {
  id: number;
  username: string;
}

export interface ShopDetail extends Shop {
  moderationNote: string | null;
  creator: ShopCreator | null;
  reviews: Review[];
}

export interface AdminShop extends Shop {
  moderationNote: string | null;
  creator: ShopCreator | null;
}

export interface ShopCreateInput {
  name: string;
}

export interface ShopUpdateInput {
  name: string;
}
