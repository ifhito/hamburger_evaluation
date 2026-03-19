export interface Review {
  id: number
  rating: number
  comment: string
  created_at: string
  user: { id: number; username: string } | null
  burger: { id: number; name: string; average_rating: number; review_count: number } | null
}

export interface ReviewCreateInput {
  rating: number
  comment: string
  shop_id: number
  burger_name: string
}

export interface ReviewUpdateInput {
  rating: number
  comment: string
}

export interface ReviewListParams {
  rating?: number
  keyword?: string
}
