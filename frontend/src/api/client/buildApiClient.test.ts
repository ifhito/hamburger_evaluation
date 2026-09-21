import { describe, it, expect } from "vitest";
import type { InternalAxiosRequestConfig } from "axios";
import { ApiError, buildApiClient } from "./buildApiClient";

describe("buildApiClient", () => {
  it("FormData はキー変換せずそのまま送る", async () => {
    const client = buildApiClient();
    let sent: unknown;
    client.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
      sent = config.data;
      return { data: {}, status: 200, statusText: "OK", headers: {}, config };
    };

    const form = new FormData();
    form.append("burger_name", "x");
    await client.post("/reviews", form);

    expect(sent).toBeInstanceOf(FormData);
    expect((sent as FormData).get("burger_name")).toBe("x");
  });

  it("通常のオブジェクトはキーを snake_case に変換して JSON で送る", async () => {
    const client = buildApiClient();
    let sent: unknown;
    client.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
      sent = config.data;
      return { data: {}, status: 200, statusText: "OK", headers: {}, config };
    };

    await client.post("/reviews", { review: { shopId: 1 } });

    // adapter に届く時点では transformRequest 済みで、JSON 文字列になっている
    expect(typeof sent).toBe("string");
    expect(JSON.parse(sent as string)).toEqual({ review: { shop_id: 1 } });
  });

  it("失敗の応答の本文は、成功の応答と同じく、キーを camelCase にして、ApiError.body に持つ(feature が snake_case を読まなくて済む)", async () => {
    const client = buildApiClient();
    client.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
      const response = { data: { errors: ["exists"], return_to: "/oauth/authorize?a=1", nested_value: { inner_key: 1 } }, status: 409, statusText: "", headers: {}, config };
      throw Object.assign(new Error("request failed"), { isAxiosError: true, response, config });
    };

    const failed = await client.post("/x", {}).catch((e: unknown) => e);

    expect(failed).toBeInstanceOf(ApiError);
    expect((failed as ApiError).messages).toEqual(["exists"]);
    expect((failed as ApiError).body).toEqual({ errors: ["exists"], returnTo: "/oauth/authorize?a=1", nestedValue: { innerKey: 1 } });
  });
});
