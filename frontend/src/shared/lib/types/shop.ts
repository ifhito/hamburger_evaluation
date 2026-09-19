import type { Review } from './review'

export interface Shop {
  id: number
  name: string
  status: string
}

export interface ShopDetail extends Shop {
  moderation_note: string | null
  creator: { id: number; username: string } | null
  reviews: Review[]
}
