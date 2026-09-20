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
});
