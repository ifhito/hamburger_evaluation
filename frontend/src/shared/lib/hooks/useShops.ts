import { useQuery } from '@tanstack/react-query'
import { shopsApi } from '../api'

export function useShops(params?: { keyword?: string }) {
  return useQuery({
    queryKey: ['shops', params],
    queryFn: () => shopsApi.list(params),
  })
}

export function useShopDetail(id: number) {
  return useQuery({
    queryKey: ['shops', id],
    queryFn: () => shopsApi.get(id),
  })
}
