import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { InternalAxiosRequestConfig } from "axios";
import { ApiError } from "../../../api/client/buildApiClient";
import { authApi, authApiClient } from "./authApiClient";

// authApiClient は axios のインスタンスなので、adapter を差し替えて、実際に送られるリクエスト
// (メソッド・パス・snake_case に変換された本文)と、応答の変換を確かめる。
describe("authApi の signup と、メールのリンクでの確認", () => {
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

describe("authApi の Google でのサインイン", () => {
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

  it("exchangeGoogleCode は POST /auth/google/exchange にコードを送り、サインインの成功を camelCase で返す", async () => {
    respondWith(200, { id: "u1", username: "carol", email: "c@example.com", admin: false, can_moderate: false, token: "jwt", return_to: "/shops" });

    const res = await authApi.exchangeGoogleCode("one-time-code");

    expect(sent.method).toBe("post");
    expect(sent.url).toBe("/auth/google/exchange");
    expect(sent.body).toEqual({ code: "one-time-code" });
    expect(res).toMatchObject({ id: "u1", token: "jwt", returnTo: "/shops", canModerate: false });
  });

  it("結び付けの成功は、linked と戻り先を返す", async () => {
    respondWith(200, { linked: true, return_to: "/users/u1" });
    expect(await authApi.exchangeGoogleCode("c")).toEqual({ linked: true, returnTo: "/users/u1" });
  });

  it("重複(409)・失敗(400)は、サーバーの文言を持つ ApiError になる", async () => {
    respondWith(409, { errors: ["An account with this email address already exists. Sign in with your password, then connect Google from your profile."] });
    const conflict = await authApi.exchangeGoogleCode("c").catch((e: unknown) => e);
    expect(conflict).toBeInstanceOf(ApiError);
    expect((conflict as ApiError).status).toBe(409);
    expect((conflict as ApiError).messages[0]).toContain("already exists");

    respondWith(400, { errors: ["Google sign-in failed. Please try again."] });
    const failed = await authApi.exchangeGoogleCode("").catch((e: unknown) => e);
    expect((failed as ApiError).status).toBe(400);
  });

  it("失敗の応答(409・400)が return_to を含むときは、ApiError の returnTo に入る(ないとき・文字列でないときは undefined)", async () => {
    respondWith(409, { errors: ["exists"], return_to: "/oauth/authorize?client_id=app-1&state=xyz" });
    const withReturn = await authApi.exchangeGoogleCode("c").catch((e: unknown) => e);
    expect((withReturn as ApiError).returnTo).toBe("/oauth/authorize?client_id=app-1&state=xyz");

    respondWith(400, { errors: ["failed"] });
    expect(((await authApi.exchangeGoogleCode("c").catch((e: unknown) => e)) as ApiError).returnTo).toBeUndefined();

    respondWith(400, { errors: ["failed"], return_to: { evil: true } });
    expect(((await authApi.exchangeGoogleCode("c").catch((e: unknown) => e)) as ApiError).returnTo).toBeUndefined();
  });

  it("startGoogleLink は POST /me/identities/google/link で、戻り先を snake_case で送り、Google の URL(redirectUrl)を返す", async () => {
    respondWith(200, { redirect_url: "https://accounts.google.com/o/oauth2/v2/auth?state=s" });
    expect(await authApi.startGoogleLink("/users/u1")).toEqual({ redirectUrl: "https://accounts.google.com/o/oauth2/v2/auth?state=s" });
    expect(sent.method).toBe("post");
    expect(sent.url).toBe("/me/identities/google/link");
    expect(sent.body).toEqual({ return_to: "/users/u1" });
  });

  it("startGoogleLink は、戻り先を省略すると、本文なしで送る(開始のコードは受け取らない)", async () => {
    respondWith(200, { redirect_url: "https://accounts.google.com/x" });
    const res = await authApi.startGoogleLink();
    expect(sent.body).toBeUndefined();
    expect(res).not.toHaveProperty("linkCode");
  });

  it("listIdentities は GET /me/identities で、結び付きの一覧を camelCase で返す", async () => {
    respondWith(200, { identities: [{ provider: "google", email: "c@gmail.example", connected_at: "2026-09-21T00:00:00Z", can_unlink: false }] });
    expect(await authApi.listIdentities()).toEqual({
      identities: [{ provider: "google", email: "c@gmail.example", connectedAt: "2026-09-21T00:00:00Z", canUnlink: false }],
    });
    expect(sent.method).toBe("get");
    expect(sent.url).toBe("/me/identities");
  });

  it("unlinkGoogle は DELETE /me/identities/google を呼び、解除できないとき(422)はサーバーの文言の ApiError になる", async () => {
    respondWith(204, "");
    await authApi.unlinkGoogle();
    expect(sent.method).toBe("delete");
    expect(sent.url).toBe("/me/identities/google");

    respondWith(422, { errors: ["Google is your only way to sign in. Add a password before disconnecting it."] });
    const error = await authApi.unlinkGoogle().catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(422);
  });
});
