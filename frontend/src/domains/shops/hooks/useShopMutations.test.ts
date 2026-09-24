import { describe, it, expect, vi } from "vitest";
import { adminShopsKey, isShopKey, useAdminShops } from "./useShopMutations";
import { useInfinitePages } from "../../../api/useInfinitePages";

vi.mock("../../../api/useInfinitePages", () => ({ useInfinitePages: vi.fn() }));

describe("管理者向け店舗一覧のページング", () => {
  it("状態とページ番号を送り、ページサイズはbackendに任せる", () => {
    expect(adminShopsKey(undefined)(0, null)).toBe("/admin/shops?page=1");
    expect(adminShopsKey("pending")(0, null)).toBe("/admin/shops?status=pending&page=1");
    expect(adminShopsKey("pending")(1, { items: [], hasMore: true })).toBe("/admin/shops?status=pending&page=2");
  });

  it("サーバーが次ページなしと返したら取得を止める", () => {
    expect(adminShopsKey("pending")(1, { items: [], hasMore: false })).toBeNull();
  });

  it("状態を切り替えると先頭ページのキーも変わる", () => {
    expect(adminShopsKey("active")(0, null)).not.toBe(adminShopsKey("pending")(0, null));
  });

  it("閲覧者が未確定なら取得せず、確定後はそのIDでキャッシュを分ける", () => {
    useAdminShops(undefined, null);
    const key = vi.mocked(useInfinitePages).mock.lastCall?.[0];
    expect(key?.(0, null)).toBeNull();
    useAdminShops("pending", "admin1");
    expect(useInfinitePages).toHaveBeenLastCalledWith(expect.any(Function), expect.any(Function), { scope: "admin1" });
  });
});

describe("isShopKey", () => {
  it("matches public shop keys", () => {
    expect(isShopKey("/shops")).toBe(true);
    expect(isShopKey("/shops?keyword=x")).toBe(true);
    expect(isShopKey("/shops/1")).toBe(true);
  });

  it("matches admin shop keys", () => {
    expect(isShopKey("/admin/shops")).toBe(true);
    expect(isShopKey("/admin/shops?status=pending")).toBe(true);
  });

  it("matches the viewer-keyed detail key", () => {
    expect(isShopKey(["/shops", 1, 3])).toBe(true);
    expect(isShopKey(["/shops", 1, null])).toBe(true);
  });

  it("does not match unrelated keys", () => {
    expect(isShopKey("/reviews")).toBe(false);
    expect(isShopKey("/users")).toBe(false);
    expect(isShopKey(123)).toBe(false);
    expect(isShopKey(null)).toBe(false);
    expect(isShopKey(["/users", 1, 3])).toBe(false);
  });
});
