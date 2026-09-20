import { describe, it, expect } from "vitest";
import type { InternalAxiosRequestConfig } from "axios";
import { buildApiClient } from "./buildApiClient";

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
});
