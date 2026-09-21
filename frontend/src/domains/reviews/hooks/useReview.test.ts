import { describe, it, expect } from "vitest";
import { reviewKey } from "./useReview";

describe("reviewKey", () => {
  it("id と viewerId を含む配列キーを返す", () => {
    expect(reviewKey(2, 3)).toEqual(["/reviews", 2, 3]);
    expect(reviewKey(2, null)).toEqual(["/reviews", 2, null]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる(can_edit が閲覧者ごとに違うため)", () => {
    expect(reviewKey(2, 3)).not.toEqual(reviewKey(2, null));
    expect(reviewKey(2, 3)).not.toEqual(reviewKey(2, 4));
  });

  it("enabled が false、または id が空なら null を返す", () => {
    expect(reviewKey(2, 3, false)).toBeNull();
    expect(reviewKey(undefined, 3)).toBeNull();
    expect(reviewKey(0, 3)).toBeNull();
    expect(reviewKey(NaN, 3)).toBeNull();
  });
});
