import { describe, it, expect } from "vitest";
import { getKey } from "./useBurgers";
import type { Page } from "../../../api/page";
import type { BurgerRanking } from "../api/types";

function page(hasMore: boolean, count = 1): Page<BurgerRanking> {
  return {
    items: Array.from({ length: count }, (_, i) => ({
      id: `00000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
      name: `Burger ${i + 1}`,
      shop: { id: "00000000-0000-4000-8000-000000000099", name: "Shop" },
      averageRating: 4,
      weightedScore: 3.9,
      reviewCount: 5,
    })),
    hasMore,
  };
}

describe("getKey", () => {
  it("先頭ページ(previous が null)は常にキーを返し、per_page は送らない", () => {
    expect(getKey(0, null)).toBe("/burgers?page=1");
  });

  it("前ページに続きがある(hasMore が true)なら次ページのキーを返す", () => {
    expect(getKey(1, page(true))).toBe("/burgers?page=2");
  });

  it("前ページが最終(hasMore が false)なら、件数によらず null を返して読み込みを止める", () => {
    expect(getKey(1, page(false, 20))).toBeNull();
    expect(getKey(1, page(false, 0))).toBeNull();
  });
});
