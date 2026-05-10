import { useSWRConfig } from "swr";
import { reviewApiClient } from "../api/reviewApiClient";
import type { Review, ReviewCreateInput, ReviewUpdateInput } from "../api/types";

function isReviewKey(key: unknown): boolean {
  return typeof key === "string" && key.startsWith("/reviews");
}

export function useCreateReview() {
  const { mutate } = useSWRConfig();
  return {
    create: async (data: ReviewCreateInput): Promise<Review> => {
      const res = await reviewApiClient.post<Review>("/reviews", { review: data });
      await mutate(isReviewKey);
      return res.data;
    },
  };
}

export function useUpdateReview(id: number) {
  const { mutate } = useSWRConfig();
  return {
    update: async (data: ReviewUpdateInput): Promise<Review> => {
      const res = await reviewApiClient.put<Review>(`/reviews/${id}`, { review: data });
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
