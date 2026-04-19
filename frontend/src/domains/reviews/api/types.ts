export interface Review {
  id: number;
  rating: number;
  comment: string;
  createdAt: string;
  user: { id: number; username: string } | null;
  burger: {
    id: number;
    name: string;
    averageRating: number;
    reviewCount: number;
    weightedScore: number;
    confidence: number;
  } | null;
}

export interface ReviewCreateInput {
  rating: number;
  comment: string;
  shopId: number;
  burgerName: string;
}

export interface ReviewUpdateInput {
  rating: number;
  comment: string;
}

export interface ReviewSearchParams {
  rating?: number;
  keyword?: string;
}
