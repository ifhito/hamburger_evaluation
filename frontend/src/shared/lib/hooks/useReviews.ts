import { useInfiniteQuery } from '@tanstack/react-query'
import { reviewsApi } from '../api'
import type { ReviewListParams } from '../types/review'

// 「返却件数が PER_PAGE 未満なら最終ページ」の判定と API へ送る per_page を一致させる
const PER_PAGE = 20

export function useReviews(params?: ReviewListParams, options?: { enabled?: boolean }) {
  return useInfiniteQuery({
    // 呼び出し側が取得を止められる。例: user_id が不正なとき、そのまま呼ぶと 422 になり、user_id を外して呼ぶと全件が返る。未指定なら常に取得する
    enabled: options?.enabled,
    // page / per_page は含めない。フィルタ(params)が変わると別キーになり、先頭ページから読み直される
    queryKey: ['reviews', params],
    queryFn: ({ pageParam }) => reviewsApi.list({ ...params, page: pageParam, per_page: PER_PAGE }),
    initialPageParam: 1,
    getNextPageParam: (lastPage, _allPages, lastPageParam) =>
      lastPage.length < PER_PAGE ? undefined : lastPageParam + 1,
    // offset ページングではページ間の新規投稿で前ページ末尾が次ページに再登場するため、id で重複を除く
    // (先頭出現の位置を保つ。削除で 1 件飛ぶ場合は解消できない既知の制約)
    select: (data) => [...new Map(data.pages.flat().map((r) => [r.id, r])).values()],
  })
}
