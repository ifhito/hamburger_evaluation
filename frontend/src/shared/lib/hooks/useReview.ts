import { useQuery } from '@tanstack/react-query'
import { reviewsApi } from '../api'

export function useReview(id: number) {
  return useQuery({
    queryKey: ['reviews', id],
    queryFn: () => reviewsApi.get(id),
    enabled: !!id,
  })
}
