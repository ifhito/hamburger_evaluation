import { describe, it, expect } from "vitest";
import { RATING_MAX, formatRating } from "./rating";

describe("formatRating", () => {
  it("rating の分だけ ★ を、残りを ☆ で埋める", () => {
    expect(formatRating(3)).toBe("★★★☆☆");
    expect(formatRating(1)).toBe("★☆☆☆☆");
    expect(formatRating(RATING_MAX)).toBe("★★★★★");
  });

  it("範囲外の値でも例外にならず、0〜RATING_MAX に丸める(範囲の判断は backend が持つ)", () => {
    expect(() => formatRating(7)).not.toThrow();
    expect(formatRating(7)).toBe("★★★★★");
    expect(formatRating(-2)).toBe("☆☆☆☆☆");
    expect(formatRating(2.9)).toBe("★★☆☆☆");
  });
});
