import { describe, it, expect } from "vitest";
import { getKey, shopDetailKey } from "./useShops";
import type { Page } from "../../../api/page";
import type { Shop } from "../api/types";

function page(hasMore: boolean, count = 1): Page<Shop> {
  return {
    items: Array.from({ length: count }, (_, i) => ({ id: i + 1, name: `Shop ${i + 1}`, status: "active" as const })),
    hasMore,
  };
}

describe("getKey", () => {
  it("先頭ページ(previous が null)は常にキーを返し、per_page は送らない", () => {
    expect(getKey(undefined)(0, null)).toBe("/shops?page=1");
  });

  it("keyword はキーに入り、空の keyword は入らない", () => {
    expect(getKey({ keyword: "beef" })(0, null)).toBe("/shops?keyword=beef&page=1");
    expect(getKey({ keyword: "" })(0, null)).toBe("/shops?page=1");
  });

  it("前ページに続きがある(hasMore が true)なら次ページのキーを返す", () => {
    expect(getKey(undefined)(1, page(true))).toBe("/shops?page=2");
  });

  it("前ページが最終(hasMore が false)なら、件数によらず null を返して読み込みを止める", () => {
    expect(getKey(undefined)(1, page(false, 20))).toBeNull();
    expect(getKey(undefined)(1, page(false, 0))).toBeNull();
  });
});

const viewer3 = "00000000-0000-4000-8000-000000000003";
const viewer4 = "00000000-0000-4000-8000-000000000004";

describe("shopDetailKey", () => {
  it("id と viewerId を含む配列キーを返す", () => {
    expect(shopDetailKey(2, viewer3)).toEqual(["/shops", 2, viewer3]);
    expect(shopDetailKey(2, null)).toEqual(["/shops", 2, null]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる", () => {
    expect(shopDetailKey(2, viewer3)).not.toEqual(shopDetailKey(2, null));
    expect(shopDetailKey(2, viewer3)).not.toEqual(shopDetailKey(2, viewer4));
  });

  it("enabled が false、または id が空なら null を返す", () => {
    expect(shopDetailKey(2, viewer3, false)).toBeNull();
    expect(shopDetailKey(undefined, viewer3)).toBeNull();
    expect(shopDetailKey(0, viewer3)).toBeNull();
    expect(shopDetailKey(NaN, viewer3)).toBeNull();
  });
});
