// @vitest-environment jsdom
import { createElement } from "react";
import { act } from "react";
import useSWR, { SWRConfig } from "swr";
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, mount } from "../../../test/dom";
import { reviewApiClient } from "../api/reviewApiClient";
import { isReviewKey, toCreateFormData, toUpdateFormData, useCreateReview, useDeleteReview } from "./useReviewMutations";

afterEach(cleanup);

const photo = new File(["x"], "burger.jpg", { type: "image/jpeg" });

describe("toCreateFormData", () => {
  it("API に合わせた snake_case のキーだけを、photo を最後にして並べる", () => {
    const form = toCreateFormData(
      { rating: 4, comment: "うまい", shopId: "00000000-0000-4000-8000-000000000007", burgerName: "チーズバーガー", visitedAt: "2026-09-01" },
      photo
    );

    expect([...form.keys()]).toEqual(["rating", "comment", "shop_id", "burger_name", "visited_at", "photo"]);
  });

  it("数値は文字列化され、photo は File のまま入る", () => {
    const form = toCreateFormData(
      { rating: 4, comment: "うまい", shopId: "00000000-0000-4000-8000-000000000007", burgerName: "チーズバーガー", visitedAt: "2026-09-01" },
      photo
    );

    expect(form.get("rating")).toBe("4");
    expect(form.get("comment")).toBe("うまい");
    expect(form.get("shop_id")).toBe("00000000-0000-4000-8000-000000000007");
    expect(form.get("burger_name")).toBe("チーズバーガー");
    expect(form.get("visited_at")).toBe("2026-09-01");
    expect(form.get("photo")).toBeInstanceOf(File);
  });
});

describe("toUpdateFormData", () => {
  it("rating / comment / visited_at / photo だけで、shop_id と burger_name は入らない", () => {
    const form = toUpdateFormData({ rating: 5, comment: "更新", visitedAt: "2026-09-02" }, photo);

    expect([...form.keys()]).toEqual(["rating", "comment", "visited_at", "photo"]);
    expect(form.get("rating")).toBe("5");
    expect(form.get("comment")).toBe("更新");
    expect(form.get("visited_at")).toBe("2026-09-02");
    expect(form.has("shop_id")).toBe(false);
    expect(form.has("burger_name")).toBe(false);
  });
});

describe("isReviewKey", () => {
  it("一覧のキー(文字列)に一致する", () => {
    expect(isReviewKey("/reviews")).toBe(true);
    expect(isReviewKey("/reviews?page=1")).toBe(true);
  });

  it("詳細のキー(viewerId を含む配列)に一致する", () => {
    expect(isReviewKey(["/reviews", 1, 3])).toBe(true);
    expect(isReviewKey(["/reviews", 1, null])).toBe(true);
  });

  it("無関係なキーには一致しない", () => {
    expect(isReviewKey("/shops")).toBe(false);
    expect(isReviewKey(["/users", 1, 3])).toBe(false);
    expect(isReviewKey(null)).toBe(false);
    expect(isReviewKey(123)).toBe(false);
  });
});

describe("レビュー書き込み時の関連キャッシュ再検証", () => {
  it("レビューを作成すると、店舗詳細やバーガーランキングの購読者も再取得され、削除時は削除対象の詳細を再取得しない", async () => {
    const postSpy = vi.spyOn(reviewApiClient, "post").mockResolvedValue({ data: { id: "r-new" } } as never);
    const deleteSpy = vi.spyOn(reviewApiClient, "delete").mockResolvedValue({ data: {} } as never);
    try {
      let shopFetches = 0;
      let rankingFetches = 0;
      let deletedDetailFetches = 0;
      let otherDetailFetches = 0;
      let createFn!: ReturnType<typeof useCreateReview>["create"];
      let destroyFn!: ReturnType<typeof useDeleteReview>["destroy"];

      function Harness() {
        useSWR(["/shops", "s1", "u1"], async () => `shop-${++shopFetches}`);
        useSWR("/burgers?page=1", async () => `rank-${++rankingFetches}`);
        useSWR(["/reviews", "r-target", "u1"], async () => `target-${++deletedDetailFetches}`);
        useSWR(["/reviews", "r-other", "u1"], async () => `other-${++otherDetailFetches}`);
        createFn = useCreateReview().create;
        destroyFn = useDeleteReview().destroy;
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
      expect(rankingFetches).toBe(1);
      expect(deletedDetailFetches).toBe(1);
      expect(otherDetailFetches).toBe(1);

      await act(async () => {
        await createFn({ rating: 5, comment: "最高", shopId: "s1", burgerName: "クラシック", visitedAt: "" });
      });
      expect(shopFetches).toBe(2);
      expect(rankingFetches).toBe(2);
      expect(deletedDetailFetches).toBe(2);

      await act(async () => {
        await destroyFn("r-target");
      });
      // 削除した r-target 自身の GET は増えず(2 のまま)、他レビュー・店舗・ランキングは 3 回目へ再検証される
      expect(deletedDetailFetches).toBe(2);
      expect(otherDetailFetches).toBe(3);
      expect(shopFetches).toBe(3);
      expect(rankingFetches).toBe(3);
    } finally {
      postSpy.mockRestore();
      deleteSpy.mockRestore();
    }
  });
});
