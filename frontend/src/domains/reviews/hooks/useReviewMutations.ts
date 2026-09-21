import { useSWRConfig } from "swr";
import { shrinkPhoto, type PhotoLimits } from "../../../lib/photoResize";
import { reviewApiClient } from "../api/reviewApiClient";
import type { ReviewCreateInput, ReviewUpdateInput, ReviewView } from "../api/types";
import { usePhotoLimits } from "./usePhotoLimits";

// 送る前に、上限を超える写真だけを縮小する。写真がない、または上限がまだ取得できていないときは、そのまま返す。
async function prepare(photo: File | null | undefined, limits: PhotoLimits | undefined) {
  return photo && limits ? shrinkPhoto(photo, limits) : photo;
}

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

export function useCreateReview() {
  const { mutate } = useSWRConfig();
  const limits = usePhotoLimits();
  return {
    create: async (data: ReviewCreateInput, photo?: File | null): Promise<ReviewView> => {
      const sending = await prepare(photo, limits);
      const body = sending ? toCreateFormData(data, sending) : { review: data };
      const res = await reviewApiClient.post<ReviewView>("/reviews", body);
      await mutate(isReviewKey);
      return res.data;
    },
  };
}

export function useUpdateReview(id: string) {
  const { mutate } = useSWRConfig();
  const limits = usePhotoLimits();
  return {
    update: async (data: ReviewUpdateInput, photo?: File | null): Promise<ReviewView> => {
      const sending = await prepare(photo, limits);
      const body = sending ? toUpdateFormData(data, sending) : { review: data };
      const res = await reviewApiClient.put<ReviewView>(`/reviews/${id}`, body);
      await mutate(isReviewKey);
      return res.data;
    },
  };
}

export function useDeleteReview() {
  const { mutate } = useSWRConfig();
  return {
    destroy: async (id: string): Promise<void> => {
      await reviewApiClient.delete(`/reviews/${id}`);
      await mutate(isReviewKey);
    },
  };
}
