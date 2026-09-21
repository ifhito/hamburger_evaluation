import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { InternalAxiosRequestConfig } from "axios";
import { ApiError } from "../../../api/client/buildApiClient";
import { authApi, authApiClient } from "./authApiClient";

// authApiClient は axios のインスタンスなので、adapter を差し替えて、実際に送られるリクエスト
// (メソッド・パス・snake_case に変換された本文)と、応答の変換を確かめる。
describe("authApi の signup の確認(S16)", () => {
  let sent: { method?: string; url?: string; body?: unknown } = {};
  const originalAdapter = authApiClient.defaults.adapter;

  const respondWith = (status: number, data: unknown) => {
    authApiClient.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
      sent = { method: config.method, url: config.url, body: config.data ? JSON.parse(config.data as string) : undefined };
      const response = { data, status, statusText: "", headers: {}, config };
      if (status >= 400) {
        throw Object.assign(new Error("request failed"), { isAxiosError: true, response, config });
      }
      return response;
    };
  };

  beforeEach(() => {
    sent = {};
    vi.stubGlobal("localStorage", { getItem: () => null });
  });
  afterEach(() => {
    authApiClient.defaults.adapter = originalAdapter;
    vi.unstubAllGlobals();
  });

  it("signup は POST /signup にキーを snake_case にして送り、202 の message を返す", async () => {
    respondWith(202, { message: "Confirmation email sent" });

    const res = await authApi.signup({
      username: "alice",
      email: "alice@example.com",
      password: "Password123!",
      passwordConfirmation: "Password123!",
    });

    expect(sent.method).toBe("post");
    expect(sent.url).toBe("/signup");
    expect(sent.body).toEqual({
      username: "alice",
      email: "alice@example.com",
      password: "Password123!",
      password_confirmation: "Password123!",
    });
    expect(res).toEqual({ message: "Confirmation email sent" });
  });

  it("confirmSignup は POST /signup/confirm に token を送り、user と token を返す", async () => {
    respondWith(201, { id: 1, username: "alice", email: "alice@example.com", admin: false, token: "jwt" });

    const res = await authApi.confirmSignup("raw-token");

    expect(sent.method).toBe("post");
    expect(sent.url).toBe("/signup/confirm");
    expect(sent.body).toEqual({ token: "raw-token" });
    expect(res).toEqual({ id: 1, username: "alice", email: "alice@example.com", admin: false, token: "jwt" });
  });

  it("無効なトークンの 400 は、サーバーのメッセージを持つ ApiError になる", async () => {
    respondWith(400, { error: "Confirmation token is invalid or has expired" });

    const error = await authApi.confirmSignup("bad").catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(400);
    expect((error as ApiError).messages).toEqual(["Confirmation token is invalid or has expired"]);
  });
});
