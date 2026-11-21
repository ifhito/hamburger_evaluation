import { describe, expect, it } from "vitest";
import { isBurgerKey, isReviewKey, isShopKey, isUserKey } from "./cacheKeys";

describe("cacheKeys(ドメイン横断の SWR キャッシュキー述語)", () => {
  it("isBurgerKey はバーガーのランキング・一覧・詳細キーだけに一致し、他ドメインには一致しない", () => {
    expect(isBurgerKey("/burgers?page=1")).toBe(true);
    expect(isBurgerKey(["/burgers", "b1", "u1"])).toBe(true);
    expect(isBurgerKey(["/burgers", "b1", null])).toBe(true);
    expect(isBurgerKey("/shops")).toBe(false);
    expect(isBurgerKey(["/reviews", "r1", null])).toBe(false);
    expect(isBurgerKey(null)).toBe(false);
  });

  it("isReviewKey・isShopKey・isUserKey はそれぞれのドメインキーだけに一致し、他ドメインには一致しない", () => {
    expect(isReviewKey("/reviews?page=1")).toBe(true);
    expect(isReviewKey(["/reviews", "r1", "u1"])).toBe(true);
    expect(isReviewKey("/shops")).toBe(false);
    expect(isReviewKey(["/burgers", "b1", null])).toBe(false);

    expect(isShopKey("/shops?page=1")).toBe(true);
    expect(isShopKey("/admin/shops?status=pending")).toBe(true);
    expect(isShopKey(["/shops", "s1", null])).toBe(true);
    expect(isShopKey("/reviews")).toBe(false);
    expect(isShopKey(["/users", "u1", null])).toBe(false);

    expect(isUserKey(["/users", "u1", null])).toBe(true);
    expect(isUserKey("/users")).toBe(false);
    expect(isUserKey(["/shops", "s1", null])).toBe(false);
  });
});
