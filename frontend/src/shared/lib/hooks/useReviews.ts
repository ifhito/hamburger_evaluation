import { useQuery } from '@tanstack/react-query'
import { reviewsApi } from '../api'
import type { ReviewListParams } from '../types/review'

export function useReviews(params?: ReviewListParams) {
  return useQuery({
    queryKey: ['reviews', params],
    queryFn: () => reviewsApi.list(params),
  })
}
