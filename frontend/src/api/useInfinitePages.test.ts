import { describe, it, expect, vi } from "vitest";
import { revalidateInfiniteLists, withScope } from "./useInfinitePages";

describe("withScope", () => {
  it("scope が違えば、同じ url でも別々のキーになる", () => {
    const url = "/shops?page=1";
    expect(withScope(url, "viewer-a")).not.toEqual(withScope(url, "viewer-b"));
    expect(withScope(url, "viewer-a")).not.toEqual(withScope(url, undefined));
    expect(withScope(url, "viewer-b")).not.toEqual(withScope(url, undefined));
  });

  it("key が null なら、scope に関係なく null のまま(読み込みを止める)", () => {
    expect(withScope(null, "viewer-a")).toBeNull();
  });

  it("scope が undefined なら、素の url をそのまま返す", () => {
    const url = "/shops?page=1";
    expect(withScope(url, undefined)).toBe(url);
  });
});

describe("revalidateInfiniteLists", () => {
  it("$inf$ で始まるキーだけを mutate し、$sub$ や素のキーは対象にしない", async () => {
    const cache = { keys: () => ["$inf$abc", "/shops?page=1", "$sub$xyz", "$inf$def"].values() };
    const mutate = vi.fn().mockResolvedValue(undefined);

    await revalidateInfiniteLists(cache, mutate);

    expect(mutate).toHaveBeenCalledTimes(2);
    expect(mutate).toHaveBeenCalledWith("$inf$abc");
    expect(mutate).toHaveBeenCalledWith("$inf$def");
  });

  it("戻り値の Promise は解決する", async () => {
    const cache = { keys: () => ["$inf$abc"].values() };
    const mutate = vi.fn().mockResolvedValue(undefined);

    await expect(revalidateInfiniteLists(cache, mutate)).resolves.toEqual([undefined]);
  });
});
