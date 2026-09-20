import { useInfiniteQuery } from '@tanstack/react-query'
import { reviewsApi } from '../api'
import type { ReviewListParams } from '../types/review'

// 「返却件数が PER_PAGE 未満なら最終ページ」の判定と API へ送る per_page を一致させる
const PER_PAGE = 20

export function useReviews(params?: ReviewListParams) {
  return useInfiniteQuery({
    // page / per_page は含めない。フィルタ(params)が変わると別キーになり、先頭ページから読み直される
    queryKey: ['reviews', params],
    queryFn: ({ pageParam }) => reviewsApi.list({ ...params, page: pageParam, per_page: PER_PAGE }),
    initialPageParam: 1,
    getNextPageParam: (lastPage, _allPages, lastPageParam) =>
      lastPage.length < PER_PAGE ? undefined : lastPageParam + 1,
    select: (data) => data.pages.flat(),
  })
}
