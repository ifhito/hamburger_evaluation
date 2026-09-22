import type { Review } from "../../reviews/api/types";

export type ShopStatus = "pending" | "active" | "rejected";

export interface Shop {
  id: string;
  name: string;
  status: ShopStatus;
  // ショップの写真(そのショップで、写真つきで最も新しいレビューの写真)。写真つきのレビューがなければ null。
  photoUrl: string | null;
  // 評価の平均(小数 1 桁)。レビューがなければ null。バーガーの平均(reviews[].burger.averageRating)とは別の値。
  averageRating: number | null;
  reviewCount: number;
}

export interface ShopCreator {
  id: string;
  username: string;
}

export interface ShopDetail extends Shop {
  moderationNote: string | null;
  creator: ShopCreator | null;
  reviews: Review[];
  // 閲覧者がこの shop にレビューを書けるか。backend が返す(匿名は false)。
  canReview: boolean;
}

export interface AdminShop extends Shop {
  moderationNote: string | null;
  creator: ShopCreator | null;
  // 承認・却下の操作を出してよいか。backend の domain が status から判断して返す(frontend は status を比較しない)。
  canApprove: boolean;
  canReject: boolean;
}

export interface ShopCreateInput {
  name: string;
}

export interface ShopUpdateInput {
  name: string;
}
