import { describe, it, expect } from "vitest";
import { reviewKey } from "./useReview";

const viewer3 = "00000000-0000-4000-8000-000000000003";
const viewer4 = "00000000-0000-4000-8000-000000000004";

describe("reviewKey", () => {
  it("id と viewerId を含む配列キーを返す", () => {
    expect(reviewKey(2, viewer3)).toEqual(["/reviews", 2, viewer3]);
    expect(reviewKey(2, null)).toEqual(["/reviews", 2, null]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる(can_edit が閲覧者ごとに違うため)", () => {
    expect(reviewKey(2, viewer3)).not.toEqual(reviewKey(2, null));
    expect(reviewKey(2, viewer3)).not.toEqual(reviewKey(2, viewer4));
  });

  it("enabled が false、または id が空なら null を返す", () => {
    expect(reviewKey(2, viewer3, false)).toBeNull();
    expect(reviewKey(undefined, viewer3)).toBeNull();
    expect(reviewKey(0, viewer3)).toBeNull();
    expect(reviewKey(NaN, viewer3)).toBeNull();
  });
});
