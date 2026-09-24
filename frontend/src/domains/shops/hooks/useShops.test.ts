import { describe, it, expect, vi, beforeEach } from "vitest";
import { getKey, shopDetailKey, useShops } from "./useShops";
import { useInfinitePages } from "../../../api/useInfinitePages";
import type { Page } from "../../../api/page";
import type { Shop } from "../api/types";

vi.mock("../../../api/useInfinitePages", () => ({
  useInfinitePages: vi.fn(() => ({
    data: undefined,
    error: undefined,
    isLoading: false,
    hasNextPage: false,
    isFetchingNextPage: false,
    refresh: vi.fn(),
    fetchNextPage: vi.fn(),
  })),
}));

function page(hasMore: boolean, count = 1): Page<Shop> {
  return {
    items: Array.from({ length: count }, (_, i) => ({
      id: `00000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
      name: `Shop ${i + 1}`,
      status: "active" as const,
      photoUrl: null,
      averageRating: null,
      reviewCount: 0,
    })),
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

  it("sort はキーに入り、指定しなければ入らない", () => {
    expect(getKey({ sort: "newest" })(0, null)).toBe("/shops?sort=newest&page=1");
    expect(getKey(undefined)(0, null)).toBe("/shops?page=1");
  });

  it("前ページに続きがある(hasMore が true)なら次ページのキーを返す", () => {
    expect(getKey(undefined)(1, page(true))).toBe("/shops?page=2");
  });

  it("前ページが最終(hasMore が false)なら、件数によらず null を返して読み込みを止める", () => {
    expect(getKey(undefined)(1, page(false, 20))).toBeNull();
    expect(getKey(undefined)(1, page(false, 0))).toBeNull();
  });

  it("enabled が false のときは、先頭ページも次ページも null を返して取得を止める", () => {
    const disabled = getKey({ keyword: "beef" }, false);

    expect(disabled(0, null)).toBeNull();
    expect(disabled(1, page(true))).toBeNull();
  });

  it("enabled を省略した場合と true の場合は、これまでどおりキーを返す", () => {
    expect(getKey({ keyword: "beef" })(0, null)).toBe("/shops?keyword=beef&page=1");
    expect(getKey({ keyword: "beef" }, true)(0, null)).toBe("/shops?keyword=beef&page=1");
  });
});

const viewer3 = "00000000-0000-4000-8000-000000000003";
const viewer4 = "00000000-0000-4000-8000-000000000004";
const shop2 = "00000000-0000-4000-8000-000000000002";

describe("shopDetailKey", () => {
  it("id と viewerId を含む配列キーを返す", () => {
    expect(shopDetailKey(shop2, viewer3)).toEqual(["/shops", shop2, viewer3]);
    expect(shopDetailKey(shop2, null)).toEqual(["/shops", shop2, null]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる", () => {
    expect(shopDetailKey(shop2, viewer3)).not.toEqual(shopDetailKey(shop2, null));
    expect(shopDetailKey(shop2, viewer3)).not.toEqual(shopDetailKey(shop2, viewer4));
  });

  it("enabled が false、または id が空なら null を返す", () => {
    expect(shopDetailKey(shop2, viewer3, false)).toBeNull();
    expect(shopDetailKey(undefined, viewer3)).toBeNull();
    expect(shopDetailKey("", viewer3)).toBeNull();
  });
});

describe("useShops", () => {
  beforeEach(() => {
    vi.mocked(useInfinitePages).mockClear();
  });

  it("閲覧者の id を useInfinitePages の scope として渡す(閲覧者ごとにキャッシュを分けるため)", () => {
    useShops(undefined, viewer3);

    expect(vi.mocked(useInfinitePages)).toHaveBeenCalledWith(
      expect.any(Function),
      expect.any(Function),
      { scope: viewer3 },
    );
  });

  it("匿名(viewerId が null)のときは scope を付けない(未ログインの誰とも重ならない素のキーになる)", () => {
    useShops(undefined, null);

    expect(vi.mocked(useInfinitePages)).toHaveBeenCalledWith(
      expect.any(Function),
      expect.any(Function),
      { scope: undefined },
    );
  });
});
