import { useSWRConfig } from "swr";
import { reviewApiClient } from "../api/reviewApiClient";
import type { Review, ReviewCreateInput, ReviewUpdateInput } from "../api/types";

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

function isReviewKey(key: unknown): boolean {
  return typeof key === "string" && key.startsWith("/reviews");
}

export function useCreateReview() {
  const { mutate } = useSWRConfig();
  return {
    create: async (data: ReviewCreateInput, photo?: File | null): Promise<Review> => {
      const body = photo ? toCreateFormData(data, photo) : { review: data };
      const res = await reviewApiClient.post<Review>("/reviews", body);
      await mutate(isReviewKey);
      return res.data;
    },
  };
}

export function useUpdateReview(id: number) {
  const { mutate } = useSWRConfig();
  return {
    update: async (data: ReviewUpdateInput, photo?: File | null): Promise<Review> => {
      const body = photo ? toUpdateFormData(data, photo) : { review: data };
      const res = await reviewApiClient.put<Review>(`/reviews/${id}`, body);
      await mutate(isReviewKey);
      return res.data;
    },
  };
}

export function useDeleteReview() {
  const { mutate } = useSWRConfig();
  return {
    destroy: async (id: number): Promise<void> => {
      await reviewApiClient.delete(`/reviews/${id}`);
      await mutate(isReviewKey);
    },
  };
}
