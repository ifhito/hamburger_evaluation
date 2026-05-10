import type { Review } from "../../reviews/api/types";

export interface ShopStat {
  averageRating: number;
  weightedScore: number;
  burgerCount: number;
  reviewCount: number;
  confidence: number;
}

export interface Shop {
  id: number;
  name: string;
  stat: ShopStat;
}

export interface ShopDetail extends Shop {
  reviews: Review[];
}
