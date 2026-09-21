import { describe, it, expect } from "vitest";
import { reviewKey } from "./useReview";

const viewer3 = "00000000-0000-4000-8000-000000000003";
const viewer4 = "00000000-0000-4000-8000-000000000004";
const review2 = "00000000-0000-4000-8000-000000000002";

describe("reviewKey", () => {
  it("id と viewerId を含む配列キーを返す", () => {
    expect(reviewKey(review2, viewer3)).toEqual(["/reviews", review2, viewer3]);
    expect(reviewKey(review2, null)).toEqual(["/reviews", review2, null]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる(can_edit が閲覧者ごとに違うため)", () => {
    expect(reviewKey(review2, viewer3)).not.toEqual(reviewKey(review2, null));
    expect(reviewKey(review2, viewer3)).not.toEqual(reviewKey(review2, viewer4));
  });

  it("enabled が false、または id が空なら null を返す", () => {
    expect(reviewKey(review2, viewer3, false)).toBeNull();
    expect(reviewKey(undefined, viewer3)).toBeNull();
    expect(reviewKey("", viewer3)).toBeNull();
  });
});
