export interface Review {
  id: number;
  rating: number;
  comment: string | null;
  photoUrl: string | null;
  createdAt: string;
  user: { id: string; username: string } | null;
  burger: {
    id: string;
    name: string;
    averageRating: number;
    reviewCount: number;
    weightedScore: number;
    confidence: number;
  } | null;
}

// /reviews 系(一覧・詳細・作成・更新)の応答。canEdit は、閲覧者がその review を編集・削除できるかで、
// backend が返す(匿名は false)。frontend は所有者の比較をせず、この値でボタンを出し分ける。
// shop 詳細に埋め込まれる Review には canEdit がない。
export interface ReviewView extends Review {
  canEdit: boolean;
}

export interface ReviewCreateInput {
  rating: number;
  comment: string;
  shopId: string;
  burgerName: string;
}

export interface ReviewUpdateInput {
  rating: number;
  comment: string;
}

export interface ReviewSearchParams {
  rating?: number;
  keyword?: string;
  userId?: string;
}
