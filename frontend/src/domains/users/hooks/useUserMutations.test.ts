import { describe, it, expect } from "vitest";
import { isUserKey } from "./useUserMutations";

describe("isUserKey", () => {
  it("useUser の配列キーに一致する", () => {
    expect(isUserKey(["/users", 2, 3])).toBe(true);
    expect(isUserKey(["/users", 2, null])).toBe(true);
  });

  it("廃止した一覧のキーや、他ドメインのキーには一致しない", () => {
    expect(isUserKey("/users")).toBe(false);
    expect(isUserKey(["/reviews"])).toBe(false);
    expect(isUserKey("/reviews?page=1")).toBe(false);
    expect(isUserKey(null)).toBe(false);
  });
});
