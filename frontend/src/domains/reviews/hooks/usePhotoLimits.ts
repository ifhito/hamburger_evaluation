import { useMeta } from "../../../api/meta";
import type { PhotoLimits } from "../../../lib/photoResize";

// 写真の上限(backend の domain の値)。取得できるまでは undefined。
export function usePhotoLimits(): PhotoLimits | undefined {
  return useMeta().data?.photo;
}
