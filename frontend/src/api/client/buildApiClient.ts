import axios from "axios";
import camelcaseKeys from "camelcase-keys";
import snakecaseKeys from "snakecase-keys";

export class ApiError extends Error {
  readonly messages: string[];
  readonly status: number;

  constructor(messages: string[], status: number) {
    super(messages[0]);
    this.name = "ApiError";
    this.messages = messages;
    this.status = status;
  }
}

export function buildApiClient(getToken?: () => string | null) {
  const client = axios.create({ baseURL: "/api" });

  client.interceptors.request.use((config) => {
    const token = getToken?.();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    if (config.data && typeof config.data === "object") {
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
        const data = error.response.data as { error?: string; errors?: string[] };
        const messages =
          data.errors ?? (data.error ? [data.error] : ["An error occurred"]);
        return Promise.reject(new ApiError(messages, error.response.status));
      }
      return Promise.reject(error);
    }
  );

  return client;
}
