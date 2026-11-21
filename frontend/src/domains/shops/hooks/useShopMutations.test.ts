// @vitest-environment jsdom
import { createElement, act } from "react";
import useSWR, { SWRConfig } from "swr";
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, mount } from "../../../test/dom";
import { shopApiClient } from "../api/shopApiClient";
import { adminShopsKey, isShopKey, useAdminShops, useUpdateShop } from "./useShopMutations";
import { useInfinitePages } from "../../../api/useInfinitePages";

vi.mock("../../../api/useInfinitePages", () => ({
  useInfinitePages: vi.fn(),
  revalidateInfiniteLists: vi.fn().mockResolvedValue([]),
}));

afterEach(cleanup);

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
  it("公開ショップの一覧キーに一致する", () => {
    expect(isShopKey("/shops")).toBe(true);
    expect(isShopKey("/shops?keyword=x")).toBe(true);
    expect(isShopKey("/shops/1")).toBe(true);
  });

  it("管理用ショップの一覧キーに一致する", () => {
    expect(isShopKey("/admin/shops")).toBe(true);
    expect(isShopKey("/admin/shops?status=pending")).toBe(true);
  });

  it("閲覧者ごとの詳細キーに一致する", () => {
    expect(isShopKey(["/shops", 1, 3])).toBe(true);
    expect(isShopKey(["/shops", 1, null])).toBe(true);
  });

  it("他ドメインのキーには一致しない", () => {
    expect(isShopKey("/reviews")).toBe(false);
    expect(isShopKey("/users")).toBe(false);
    expect(isShopKey(123)).toBe(false);
    expect(isShopKey(null)).toBe(false);
    expect(isShopKey(["/users", 1, 3])).toBe(false);
  });
});

describe("店舗更新時の関連キャッシュ再検証", () => {
  it("店舗を更新すると、店舗詳細だけでなくレビュー詳細やバーガーランキングも再検証される", async () => {
    const putSpy = vi.spyOn(shopApiClient, "put").mockResolvedValue({ data: { id: "s1" } } as never);
    try {
      let shopFetches = 0;
      let reviewFetches = 0;
      let rankingFetches = 0;
      let updateFn!: ReturnType<typeof useUpdateShop>["update"];

      function Harness() {
        useSWR(["/shops", "s1", "u1"], async () => `shop-${++shopFetches}`);
        useSWR(["/reviews", "r1", "u1"], async () => `rev-${++reviewFetches}`);
        useSWR("/burgers?page=1", async () => `rank-${++rankingFetches}`);
        updateFn = useUpdateShop("s1").update;
        return null;
      }

      await mount(
        createElement(
          SWRConfig,
          { value: { provider: () => new Map(), dedupingInterval: 0 } },
          createElement(Harness),
        ),
      );
      expect(shopFetches).toBe(1);
      expect(reviewFetches).toBe(1);
      expect(rankingFetches).toBe(1);

      await act(async () => {
        await updateFn({ name: "新しい店名", mapUrl: "" });
      });
      expect(shopFetches).toBe(2);
      expect(reviewFetches).toBe(2);
      expect(rankingFetches).toBe(2);
    } finally {
      putSpy.mockRestore();
    }
  });
});
