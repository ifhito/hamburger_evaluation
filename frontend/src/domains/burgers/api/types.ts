// GET /burgers/{id} の応答。averageRating・weightedScore・reviewCount は、レビューが 1 件もないときだけ、
// そろって null になる(0 件とは区別する)。shops は、そのバーガーが紐づくショップ(0 件でも空配列)。
export interface BurgerDetail {
  id: string;
  name: string;
  shops: { id: string; name: string }[];
  averageRating: number | null;
  weightedScore: number | null;
  reviewCount: number | null;
}

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
