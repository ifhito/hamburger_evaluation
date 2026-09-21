import { describe, it, expect } from "vitest";
import { userKey } from "./useUser";

const aliceId = "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10";
const bobId = "5d2c8a91-3f47-4e6b-8c15-7a0e9b4d2f63";

describe("userKey", () => {
  it("id があり enabled なら、id と viewerId を含む配列キーを返す", () => {
    expect(userKey(aliceId, bobId)).toEqual(["/users", aliceId, bobId]);
    expect(userKey(aliceId, null, true)).toEqual(["/users", aliceId, null]);
  });

  it("enabled が false なら null を返す", () => {
    expect(userKey(aliceId, bobId, false)).toBeNull();
  });

  it("id がなければ null を返す(形式の判定はせず、正規形でない id は API の 404 に任せる)", () => {
    expect(userKey(undefined, bobId)).toBeNull();
    expect(userKey("", bobId)).toBeNull();
    expect(userKey("abc", bobId)).toEqual(["/users", "abc", bobId]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる", () => {
    expect(userKey(aliceId, bobId)).not.toEqual(userKey(aliceId, null));
    expect(userKey(aliceId, bobId)).not.toEqual(userKey(aliceId, aliceId));
  });
});
