import useSWR from "swr";
import { reviewApiClient } from "../api/reviewApiClient";
import type { Review, ReviewSearchParams } from "../api/types";

function buildKey(params?: ReviewSearchParams): string {
  if (!params) return "/reviews";
  const entries = Object.entries(params).filter(([, v]) => v !== undefined);
  if (entries.length === 0) return "/reviews";
  const qs = new URLSearchParams(
    entries.map(([k, v]) => [k, String(v)])
  ).toString();
  return `/reviews?${qs}`;
}

export function useReviews(params?: ReviewSearchParams) {
  return useSWR<Review[]>(buildKey(params), async (url: string) => {
    const res = await reviewApiClient.get<Review[]>(url);
    if (!Array.isArray(res.data)) {
      throw new Error("Invalid response: expected array");
    }
    return res.data;
  });
}
