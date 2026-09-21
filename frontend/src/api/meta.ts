import useSWRImmutable from "swr/immutable";
import { buildApiClient } from "./client/buildApiClient";

// backend が返す、描画に使う domain のルールの値(GET /meta)。ルールを持つのは backend だけで、
// frontend は値を複製せず、これを表示・選択肢の生成に使う。
export interface Meta {
  rating: { min: number; max: number };
}

const metaApiClient = buildApiClient();

// 値はデプロイでしか変わらないので、取得は 1 回だけで、画面をまたいで共有する。
export function useMeta() {
  return useSWRImmutable<Meta>("/meta", async () => {
    const res = await metaApiClient.get<Meta>("/meta");
    return res.data;
  });
}
