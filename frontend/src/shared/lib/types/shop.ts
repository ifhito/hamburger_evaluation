import type { Review } from './review'

export interface Shop {
  id: number
  name: string
}

export interface ShopDetail extends Shop {
  reviews: Review[]
}
