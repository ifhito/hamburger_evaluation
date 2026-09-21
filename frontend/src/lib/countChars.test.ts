import { describe, it, expect } from "vitest";
import { countChars } from "./countChars";

describe("countChars", () => {
  it("英数字・日本語・絵文字を、コードポイント 1 つにつき 1 文字と数える(UTF-16 の長さでは数えない)", () => {
    expect(countChars("abc")).toBe(3);
    expect(countChars("あいう")).toBe(3);
    expect(countChars("🍔")).toBe(1);
    expect("🍔".length).toBe(2);
    expect(countChars("あ🍔a")).toBe(3);
  });

  it("空文字列は 0 文字で、改行も 1 文字と数える", () => {
    expect(countChars("")).toBe(0);
    expect(countChars("a\nb")).toBe(3);
  });

  it("絵文字をつなぐ文字(ゼロ幅接合子)を含む並びは、backend と同じくコードポイントの数で数える", () => {
    expect(countChars("👨‍👩‍👧")).toBe(5);
  });
});
