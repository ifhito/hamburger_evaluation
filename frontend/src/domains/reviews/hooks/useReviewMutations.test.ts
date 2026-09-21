import { describe, it, expect } from "vitest";
import { isReviewKey, toCreateFormData, toUpdateFormData } from "./useReviewMutations";

const photo = new File(["x"], "burger.jpg", { type: "image/jpeg" });

describe("toCreateFormData", () => {
  it("API に合わせた snake_case のキーだけを、photo を最後にして並べる", () => {
    const form = toCreateFormData(
      { rating: 4, comment: "うまい", shopId: 7, burgerName: "チーズバーガー" },
      photo
    );

    expect([...form.keys()]).toEqual(["rating", "comment", "shop_id", "burger_name", "photo"]);
  });

  it("数値は文字列化され、photo は File のまま入る", () => {
    const form = toCreateFormData(
      { rating: 4, comment: "うまい", shopId: 7, burgerName: "チーズバーガー" },
      photo
    );

    expect(form.get("rating")).toBe("4");
    expect(form.get("comment")).toBe("うまい");
    expect(form.get("shop_id")).toBe("7");
    expect(form.get("burger_name")).toBe("チーズバーガー");
    expect(form.get("photo")).toBeInstanceOf(File);
  });
});

describe("toUpdateFormData", () => {
  it("rating / comment / photo だけで、shop_id と burger_name は入らない", () => {
    const form = toUpdateFormData({ rating: 5, comment: "更新" }, photo);

    expect([...form.keys()]).toEqual(["rating", "comment", "photo"]);
    expect(form.get("rating")).toBe("5");
    expect(form.get("comment")).toBe("更新");
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
