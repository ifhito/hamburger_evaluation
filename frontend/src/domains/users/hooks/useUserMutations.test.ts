import { describe, it, expect } from "vitest";
import { isUserKey } from "./useUserMutations";

describe("isUserKey", () => {
  it("useUser の配列キーに一致する", () => {
    expect(isUserKey(["/users", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", "5d2c8a91-3f47-4e6b-8c15-7a0e9b4d2f63"])).toBe(true);
    expect(isUserKey(["/users", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", null])).toBe(true);
  });

  it("廃止した一覧のキーや、他ドメインのキーには一致しない", () => {
    expect(isUserKey("/users")).toBe(false);
    expect(isUserKey(["/reviews"])).toBe(false);
    expect(isUserKey("/reviews?page=1")).toBe(false);
    expect(isUserKey(null)).toBe(false);
  });
});
