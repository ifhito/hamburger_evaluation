import type { Review } from "../../reviews/api/types";

export interface Shop {
  id: number;
  name: string;
}

export interface ShopDetail extends Shop {
  reviews: Review[];
}
