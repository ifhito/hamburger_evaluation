import { describe, it, expect } from "vitest";
import { PER_PAGE, hasNextPage, mergePages } from "./useReviews";
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
