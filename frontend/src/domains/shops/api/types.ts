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
  // 地図へのリンク(Google マップの共有リンクなど)。未設定なら null。
  mapUrl: string | null;
  // 閉業した日時(ISO8601)。閉業していなければ null。status(審査の状態)とは独立した値。
  closedAt: string | null;
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
  // 承認・却下・閉業・再開の操作を出してよいか。backend の domain が判断して返す(frontend は status・closedAt を比較しない)。
  canApprove: boolean;
  canReject: boolean;
  canClose: boolean;
  canReopen: boolean;
}

export interface ShopCreateInput {
  name: string;
  mapUrl: string;
}

export interface ShopUpdateInput {
  name: string;
  mapUrl: string;
}
