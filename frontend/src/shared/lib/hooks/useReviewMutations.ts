import { useMutation, useQueryClient } from '@tanstack/react-query'
import { reviewsApi } from '../api'
import type { ReviewCreateInput, ReviewUpdateInput } from '../types/review'

export function useCreateReview() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: ReviewCreateInput) => reviewsApi.create(data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['reviews'] })
    },
  })
}

export function useUpdateReview(id: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: ReviewUpdateInput) => reviewsApi.update(id, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['reviews'] })
    },
  })
}

export function useDeleteReview() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => reviewsApi.delete(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['reviews'] })
    },
  })
}
