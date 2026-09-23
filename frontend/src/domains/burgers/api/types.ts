// GET /burgers の一覧(ランキング)に現れる 1 件。review が 1 件もないバーガーは backend 側で除外済みなので、
// averageRating・weightedScore・reviewCount は常に値を持つ(GET /burgers/{id} の nullable な統計とは違う)。
export interface BurgerRanking {
  id: string;
  name: string;
  shop: { id: string; name: string };
  averageRating: number;
  weightedScore: number;
  reviewCount: number;
}
