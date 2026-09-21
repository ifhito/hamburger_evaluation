import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { InternalAxiosRequestConfig } from "axios";
import { ApiError } from "../../../api/client/buildApiClient";
import { grantsUrl, oauthApi, oauthApiClient } from "./oauthApiClient";

// oauthApiClient は axios のインスタンスなので、adapter を差し替えて、実際に送られるリクエスト
// (メソッド・パス・snake_case に変換された本文)と、応答の変換を確かめる。
describe("oauthApi", () => {
  let sent: { method?: string; url?: string; body?: unknown; signal?: unknown } = {};
  const originalAdapter = oauthApiClient.defaults.adapter;

  const respondWith = (status: number, data: unknown, headers: Record<string, string> = {}) => {
    oauthApiClient.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
      sent = { method: config.method, url: config.url, body: config.data ? JSON.parse(config.data as string) : undefined, signal: config.signal };
      const response = { data, status, statusText: "", headers, config };
      if (status >= 400) {
        throw Object.assign(new Error("request failed"), { isAxiosError: true, response, config });
      }
      return response;
    };
  };

  beforeEach(() => {
    sent = {};
    vi.stubGlobal("localStorage", { getItem: () => "jwt" });
  });
  afterEach(() => {
    oauthApiClient.defaults.adapter = originalAdapter;
    vi.unstubAllGlobals();
  });

  const search = "?response_type=code&client_id=app&redirect_uri=http%3A%2F%2F127.0.0.1%2Fcb&state=abcdefgh";

  it("describe は、画面の URL の ? 以降を変えずに GET /oauth/authorize/request へ付け、応答のキーを camelCase にする", async () => {
    respondWith(200, {
      client: { id: "app", name: "Dev App" },
      scopes: [{ name: "hamburger:read", description: "View shops" }],
      consent_required: true,
    });

    const res = await oauthApi.describe(search);

    expect(sent.method).toBe("get");
    expect(sent.url).toBe(`/oauth/authorize/request${search}`);
    expect(res).toEqual({
      client: { id: "app", name: "Dev App" },
      scopes: [{ name: "hamburger:read", description: "View shops" }],
      consentRequired: true,
    });
  });

  it("describe は、? のない値でも、同じ URL になる", async () => {
    respondWith(200, { client: { id: "a", name: "a" }, scopes: [], consent_required: false });
    await oauthApi.describe(search.slice(1));
    expect(sent.url).toBe(`/oauth/authorize/request${search}`);
  });

  it("decide は、? を除いた値と approve を POST /oauth/authorize/decision に送り、戻り先を受け取る", async () => {
    respondWith(200, { redirect_to: "http://127.0.0.1/cb?code=abc&state=abcdefgh&iss=http%3A%2F%2Flocalhost%3A8080" });

    const res = await oauthApi.decide(search, true);

    expect(sent.method).toBe("post");
    expect(sent.url).toBe("/oauth/authorize/decision");
    expect(sent.body).toEqual({ query: search.slice(1), approve: true });
    expect(res.redirectTo).toBe("http://127.0.0.1/cb?code=abc&state=abcdefgh&iss=http%3A%2F%2Flocalhost%3A8080");
  });

  it("decide は、拒否のとき approve を false で送る", async () => {
    respondWith(200, { redirect_to: "http://127.0.0.1/cb?error=access_denied" });
    await oauthApi.decide(search, false);
    expect(sent.body).toEqual({ query: search.slice(1), approve: false });
  });

  it("アプリへ結果を戻せない不正な要求の 422 は、サーバーのメッセージを持つ ApiError になる", async () => {
    respondWith(422, { error: "oauth authorization request is invalid: redirect_uri does not match" });

    const error = await oauthApi.describe(search).catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(422);
    expect((error as ApiError).messages).toEqual(["oauth authorization request is invalid: redirect_uri does not match"]);
  });

  it("listApps は、ページの URL(件数は送らない)を GET し、キーを camelCase にして、続きがあるかを X-Has-More から返す", async () => {
    respondWith(
      200,
      [
        {
          id: "g1",
          client_id: "app",
          client_name: "Dev App",
          scopes: [{ name: "hamburger:read", description: "View shops" }],
          created_at: "2030-01-01T00:00:00Z",
          updated_at: "2030-01-02T00:00:00Z",
        },
      ],
      { "x-has-more": "true" },
    );

    const res = await oauthApi.listApps(grantsUrl(2));

    expect(sent.method).toBe("get");
    expect(sent.url).toBe("/oauth/grants?page=2");
    expect(res).toEqual({
      items: [
        {
          id: "g1",
          clientId: "app",
          clientName: "Dev App",
          scopes: [{ name: "hamburger:read", description: "View shops" }],
          createdAt: "2030-01-01T00:00:00Z",
          updatedAt: "2030-01-02T00:00:00Z",
        },
      ],
      hasMore: true,
    });
  });

  it("listApps は、X-Has-More が false なら、続きなしとして返す", async () => {
    respondWith(200, [], { "x-has-more": "false" });
    expect(await oauthApi.listApps(grantsUrl(1))).toEqual({ items: [], hasMore: false });
  });

  it("listApps は、X-Has-More がないとき(プロキシに落とされたなど)、黙って続けず失敗する", async () => {
    respondWith(200, []);
    await expect(oauthApi.listApps(grantsUrl(1))).rejects.toThrow("X-Has-More");
  });

  it("describe と decide は、渡された AbortSignal を、リクエストに付ける(待たなくなった取得を取り消せる)", async () => {
    const controller = new AbortController();
    respondWith(200, { client: { id: "a", name: "a" }, scopes: [], consent_required: true });
    await oauthApi.describe(search, controller.signal);
    expect(sent.signal).toBe(controller.signal);

    respondWith(200, { redirect_to: "http://127.0.0.1/cb" });
    await oauthApi.decide(search, true, controller.signal);
    expect(sent.signal).toBe(controller.signal);
  });

  it("revokeApp は DELETE /oauth/grants/{id} を呼ぶ", async () => {
    respondWith(204, "");
    await oauthApi.revokeApp("g1");
    expect(sent.method).toBe("delete");
    expect(sent.url).toBe("/oauth/grants/g1");
  });

  it("見つからない許可の取り消し(404)は、ApiError になる", async () => {
    respondWith(404, { error: "not found" });
    const error = await oauthApi.revokeApp("nope").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(404);
  });
});
