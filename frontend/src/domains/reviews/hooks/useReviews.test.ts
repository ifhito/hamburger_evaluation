import { describe, it, expect } from "vitest";
import { getKey } from "./useReviews";
import type { Page } from "../../../api/page";
import type { ReviewView } from "../api/types";

function review(id: number): ReviewView {
  return {
    id,
    rating: 3,
    comment: null,
    photoUrl: null,
    createdAt: "2026-01-01T00:00:00Z",
    user: null,
    burger: null,
    canEdit: false,
  };
}

// ページの件数はテストの関心ではない。続きがあるかは hasMore(backend の判断)だけで決まる
function page(hasMore: boolean, count = 1): Page<ReviewView> {
  return { items: Array.from({ length: count }, (_, i) => review(i + 1)), hasMore };
}

const userId = "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10";

describe("getKey", () => {
  it("前ページに続きがある(hasMore が true)なら次ページのキーを返す", () => {
    expect(getKey(undefined)(1, page(true))).toBe("/reviews?page=2");
  });

  it("前ページが最終(hasMore が false)なら、件数によらず null を返して読み込みを止める", () => {
    expect(getKey(undefined)(1, page(false, 20))).toBeNull();
    expect(getKey(undefined)(1, page(false, 0))).toBeNull();
  });

  it("件数が少なくても、hasMore が true なら続きを読み込む(件数から最終ページを推測しない)", () => {
    expect(getKey(undefined)(1, page(true, 1))).toBe("/reviews?page=2");
  });

  it("先頭ページ(previous が null)は常にキーを返す", () => {
    expect(getKey(undefined)(0, null)).toBe("/reviews?page=1");
  });

  it("per_page は送らない(1 ページの件数は backend が決める)", () => {
    expect(getKey(undefined)(0, null)).not.toContain("per_page");
    expect(getKey({ userId, keyword: "a", rating: 3 })(1, page(true))).not.toContain("per_page");
  });

  it("userId は snake_case の user_id としてキーに入る", () => {
    expect(getKey({ userId })(0, null)).toBe(`/reviews?user_id=${userId}&page=1`);
  });

  it("空の keyword はキーに含まれない", () => {
    expect(getKey({ keyword: "" })(0, null)).not.toContain("keyword");
  });

  it("index が 1 のとき page=2 になる", () => {
    const key = getKey(undefined)(1, page(true));

    expect(new URLSearchParams(key?.split("?")[1]).get("page")).toBe("2");
  });

  it("enabled が false のときは、先頭ページも次ページも null を返して取得を止める", () => {
    const disabled = getKey({ userId }, false);

    expect(disabled(0, null)).toBeNull();
    expect(disabled(1, page(true))).toBeNull();
  });

  it("enabled を省略した場合と true の場合は、これまでどおりキーを返す", () => {
    expect(getKey({ userId })(0, null)).toBe(`/reviews?user_id=${userId}&page=1`);
    expect(getKey({ userId }, true)(0, null)).toBe(`/reviews?user_id=${userId}&page=1`);
  });
});
