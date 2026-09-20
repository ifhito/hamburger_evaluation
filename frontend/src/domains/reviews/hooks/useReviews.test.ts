import { describe, it, expect } from "vitest";
import { PER_PAGE, getKey, hasNextPage, mergePages } from "./useReviews";
import type { Review } from "../api/types";

function review(id: number): Review {
  return {
    id,
    rating: 3,
    comment: null,
    photoUrl: null,
    createdAt: "2026-01-01T00:00:00Z",
    user: null,
    burger: null,
  };
}

function page(from: number, count: number): Review[] {
  return Array.from({ length: count }, (_, i) => review(from + i));
}

describe("mergePages", () => {
  it("ページをつないで 1 つの一覧にする", () => {
    const merged = mergePages([page(1, 3), page(4, 2)]);

    expect(merged.map((r) => r.id)).toEqual([1, 2, 3, 4, 5]);
  });

  it("前ページ末尾が次ページに再登場しても、先頭出現の位置で 1 件にする", () => {
    const merged = mergePages([[review(1), review(2), review(3)], [review(3), review(4)]]);

    expect(merged.map((r) => r.id)).toEqual([1, 2, 3, 4]);
  });
});

describe("hasNextPage", () => {
  it("読み込み前は次ページなし", () => {
    expect(hasNextPage(undefined)).toBe(false);
    expect(hasNextPage([])).toBe(false);
  });

  it("直近のページが PER_PAGE 件そろっていれば次ページあり", () => {
    expect(hasNextPage([page(1, PER_PAGE)])).toBe(true);
  });

  it("直近のページが PER_PAGE 件未満なら次ページなし(全件読み込み済み)", () => {
    expect(hasNextPage([page(1, PER_PAGE), page(PER_PAGE + 1, 5)])).toBe(false);
  });
});

describe("getKey", () => {
  it("前ページが PER_PAGE 件そろっていれば次ページのキーを返す", () => {
    expect(getKey(undefined)(1, page(1, PER_PAGE))).toBe("/reviews?page=2&per_page=20");
  });

  it("前ページが PER_PAGE 件未満(19 件)なら null を返して読み込みを止める", () => {
    expect(getKey(undefined)(1, page(1, PER_PAGE - 1))).toBeNull();
  });

  it("先頭ページ(previous が null)は常にキーを返す", () => {
    expect(getKey(undefined)(0, null)).toBe("/reviews?page=1&per_page=20");
  });

  it("userId は snake_case の user_id としてキーに入る", () => {
    expect(getKey({ userId: 2 })(0, null)).toBe("/reviews?user_id=2&page=1&per_page=20");
  });

  it("空の keyword はキーに含まれない", () => {
    expect(getKey({ keyword: "" })(0, null)).not.toContain("keyword");
  });

  it("index が 1 のとき page=2 になる", () => {
    const key = getKey(undefined)(1, page(1, PER_PAGE));

    // "per_page=20" が "page=2" を部分一致で含むため、文字列ではなくクエリのパラメータ単位で比べる
    expect(new URLSearchParams(key?.split("?")[1]).get("page")).toBe("2");
  });

  it("enabled が false のときは、先頭ページも次ページも null を返して取得を止める", () => {
    const disabled = getKey({ userId: 2 }, false);

    expect(disabled(0, null)).toBeNull();
    expect(disabled(1, page(1, PER_PAGE))).toBeNull();
  });

  it("enabled を省略した場合と true の場合は、これまでどおりキーを返す", () => {
    expect(getKey({ userId: 2 })(0, null)).toBe("/reviews?user_id=2&page=1&per_page=20");
    expect(getKey({ userId: 2 }, true)(0, null)).toBe("/reviews?user_id=2&page=1&per_page=20");
  });
});
