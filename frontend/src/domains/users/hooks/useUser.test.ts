import { describe, it, expect } from "vitest";
import { isValidUserId, userKey } from "./useUser";

describe("isValidUserId", () => {
  it("正の整数は有効", () => {
    expect(isValidUserId(1)).toBe(true);
    expect(isValidUserId(42)).toBe(true);
    expect(isValidUserId(Number.MAX_SAFE_INTEGER)).toBe(true);
  });

  it("NaN・0・負数・小数・安全でない整数は無効", () => {
    expect(isValidUserId(NaN)).toBe(false);
    expect(isValidUserId(0)).toBe(false);
    expect(isValidUserId(-1)).toBe(false);
    expect(isValidUserId(1.5)).toBe(false);
    expect(isValidUserId(Number.MAX_SAFE_INTEGER + 1)).toBe(false);
  });
});

describe("userKey", () => {
  it("有効な id と enabled なら、id と viewerId を含む配列キーを返す", () => {
    expect(userKey(2, 3)).toEqual(["/users", 2, 3]);
    expect(userKey(2, null, true)).toEqual(["/users", 2, null]);
  });

  it("enabled が false なら null を返す", () => {
    expect(userKey(2, 3, false)).toBeNull();
  });

  it("不正な id なら null を返す", () => {
    expect(userKey(NaN, 3)).toBeNull();
    expect(userKey(0, 3)).toBeNull();
    expect(userKey(-1, 3)).toBeNull();
    expect(userKey(1.5, 3)).toBeNull();
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる", () => {
    expect(userKey(2, 3)).not.toEqual(userKey(2, null));
    expect(userKey(2, 3)).not.toEqual(userKey(2, 4));
  });
});
