import { useMeta } from "../../../api/meta";

// rating の範囲(backend の domain の値)。取得できるまでは undefined。
export function useRatingRange(): { min: number; max: number } | undefined {
  return useMeta().data?.rating;
}
