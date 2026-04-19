import useSWR from "swr";
import { reviewApiClient } from "../api/reviewApiClient";
import type { Review } from "../api/types";

export function useReview(id: number | undefined) {
  return useSWR<Review>(
    id ? `/reviews/${id}` : null,
    async (url: string) => {
      const res = await reviewApiClient.get<Review>(url);
      return res.data;
    }
  );
}
