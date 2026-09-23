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
