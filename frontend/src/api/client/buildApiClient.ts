import axios from "axios";
import camelcaseKeys from "camelcase-keys";
import snakecaseKeys from "snakecase-keys";
import i18n from "../../lib/i18n";

export class ApiError extends Error {
  readonly messages: string[];
  readonly status: number;
  // 失敗の応答の本文(errors・error 以外の項目を読みたい呼び出し側のため。解釈は、呼び出し側が行う)。
  readonly body: unknown;

  constructor(messages: string[], status: number, body?: unknown) {
    super(messages[0]);
    this.name = "ApiError";
    this.messages = messages;
    this.status = status;
    this.body = body;
  }
}

// API の URL の根(開発では、Vite のプロキシを通す "/api")。axios のほかに、ブラウザが API へ直接移動する
// とき(Google のサインインの開始など)にも使う。
export const API_BASE_URL: string = import.meta.env.VITE_API_BASE_URL ?? "/api";

export function buildApiClient(getToken?: () => string | null) {
  const client = axios.create({ baseURL: API_BASE_URL });

  client.interceptors.request.use((config) => {
    // 選んだ言語(lib/i18n.ts)を、すべての要求に送る(R6・AC7)。S51 で、backend がこの言語でエラー文言を返す。
    config.headers["Accept-Language"] = i18n.language;
    const token = getToken?.();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    // 写真つきの multipart(FormData)はキー変換の対象外。オブジェクトとして変換すると壊れる。
    if (
      config.data &&
      typeof config.data === "object" &&
      !(config.data instanceof FormData)
    ) {
      config.data = snakecaseKeys(config.data as Record<string, unknown>, {
        deep: true,
      });
    }
    return config;
  });

  client.interceptors.response.use(
    (response) => {
      if (response.data && typeof response.data === "object") {
        response.data = camelcaseKeys(
          response.data as Record<string, unknown>,
          { deep: true }
        );
      } else if (
        typeof response.data === "string" &&
        response.data.trimStart().startsWith("<")
      ) {
        // HTML fallback page received instead of JSON (proxy/backend unreachable)
        return Promise.reject(
          new ApiError(["Backend service unavailable"], 503)
        );
      }
      return response;
    },
    (error: unknown) => {
      if (axios.isAxiosError(error) && error.response) {
        // 本文が空・null・文字列(プロキシの HTML など)のこともある(そのときは、項目なしとして扱う)。
        const data = (error.response.data ?? {}) as { error?: string; errors?: string[] };
        const messages =
          data.errors ?? (data.error ? [data.error] : ["An error occurred"]);
        // 本文の項目(errors・error 以外)も、成功の応答と同じく、camelCase にして持つ(読む側が、snake_case を知らなくて済む)。
        const body = camelcaseKeys(data as Record<string, unknown>, { deep: true });
        return Promise.reject(new ApiError(messages, error.response.status, body));
      }
      return Promise.reject(error);
    }
  );

  return client;
}
