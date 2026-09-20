import { describe, it, expect } from "vitest";
import { isShopKey } from "./useShopMutations";

describe("isShopKey", () => {
  it("matches public shop keys", () => {
    expect(isShopKey("/shops")).toBe(true);
    expect(isShopKey("/shops?keyword=x")).toBe(true);
    expect(isShopKey("/shops/1")).toBe(true);
  });

  it("matches admin shop keys", () => {
    expect(isShopKey("/admin/shops")).toBe(true);
    expect(isShopKey("/admin/shops?status=pending")).toBe(true);
  });

  it("does not match unrelated keys", () => {
    expect(isShopKey("/reviews")).toBe(false);
    expect(isShopKey("/users")).toBe(false);
    expect(isShopKey(123)).toBe(false);
    expect(isShopKey(null)).toBe(false);
  });
});
