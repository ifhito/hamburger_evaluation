import { useSWRConfig } from "swr";
import { reviewApiClient } from "../api/reviewApiClient";
import { revalidateInfiniteLists } from "../../../api/useInfinitePages";
import type { ReviewCreateInput, ReviewUpdateInput, ReviewView } from "../api/types";

// multipart のフィールド名は API に合わせた snake_case。JSON と違い自動変換は掛からない。
function toFormData(fields: Record<string, string | number>, photo: File): FormData {
  const form = new FormData();
  for (const [key, value] of Object.entries(fields)) form.append(key, String(value));
  form.append("photo", photo);
  return form;
}

// 新規作成の multipart。photo は最後に付ける。
export function toCreateFormData(data: ReviewCreateInput, photo: File): FormData {
  return toFormData(
    {
      rating: data.rating,
      comment: data.comment,
      shop_id: data.shopId,
      burger_name: data.burgerName,
    },
    photo
  );
}

// 更新の multipart。shop_id / burger_name は更新では送らない。
export function toUpdateFormData(data: ReviewUpdateInput, photo: File): FormData {
  return toFormData({ rating: data.rating, comment: data.comment }, photo);
}

// 一覧のキー(文字列 "/reviews?…")と、詳細のキー(["/reviews", id, viewerId])の両方に一致する
export function isReviewKey(key: unknown): boolean {
  return (
    (typeof key === "string" && key.startsWith("/reviews")) ||
    (Array.isArray(key) && key[0] === "/reviews")
  );
}

// photo は、渡された時点ですでに送ってよい大きさになっている前提(呼び出し側の PhotoField が、選んだ時点で
// lib/photoResize の shrinkPhoto を通す)。ここでもう一度縮小すると、同じ写真を二重にデコードすることになるため、
// 縮小はしない(PhotoField を経由しない呼び出しが増えたら、そのときに再検討する)。
export function useCreateReview() {
  const { cache, mutate } = useSWRConfig();
  return {
    create: async (data: ReviewCreateInput, photo?: File | null): Promise<ReviewView> => {
      const body = photo ? toCreateFormData(data, photo) : { review: data };
      const res = await reviewApiClient.post<ReviewView>("/reviews", body);
      await Promise.all([mutate(isReviewKey), revalidateInfiniteLists(cache, mutate)]);
      return res.data;
    },
  };
}

export function useUpdateReview(id: string) {
  const { cache, mutate } = useSWRConfig();
  return {
    update: async (data: ReviewUpdateInput, photo?: File | null): Promise<ReviewView> => {
      const body = photo ? toUpdateFormData(data, photo) : { review: data };
      const res = await reviewApiClient.put<ReviewView>(`/reviews/${id}`, body);
      await Promise.all([mutate(isReviewKey), revalidateInfiniteLists(cache, mutate)]);
      return res.data;
    },
  };
}

export function useDeleteReview() {
  const { cache, mutate } = useSWRConfig();
  return {
    destroy: async (id: string): Promise<void> => {
      await reviewApiClient.delete(`/reviews/${id}`);
      await Promise.all([mutate(isReviewKey), revalidateInfiniteLists(cache, mutate)]);
    },
  };
}
