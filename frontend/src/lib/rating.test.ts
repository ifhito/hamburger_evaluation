import { describe, it, expect } from "vitest";
import { formatRating } from "./rating";

describe("formatRating", () => {
  it("rating の分だけ ★ を、max までの残りを ☆ で埋める", () => {
    expect(formatRating(3, 5)).toBe("★★★☆☆");
    expect(formatRating(1, 5)).toBe("★☆☆☆☆");
    expect(formatRating(5, 5)).toBe("★★★★★");
  });

  it("max は引数で決まる(範囲の値を frontend が持たない)", () => {
    expect(formatRating(3, 10)).toBe("★★★☆☆☆☆☆☆☆");
    expect(formatRating(2, 3)).toBe("★★☆");
  });

  it("max が分かるまでは、空の星を描かず、rating の分の ★ だけを返す", () => {
    expect(formatRating(3, undefined)).toBe("★★★");
  });

  it("範囲外の値でも例外にならず、0〜max に丸める(範囲の判断は backend が持つ)", () => {
    expect(() => formatRating(7, 5)).not.toThrow();
    expect(formatRating(7, 5)).toBe("★★★★★");
    expect(formatRating(-2, 5)).toBe("☆☆☆☆☆");
    expect(formatRating(2.9, 5)).toBe("★★☆☆☆");
  });
});
