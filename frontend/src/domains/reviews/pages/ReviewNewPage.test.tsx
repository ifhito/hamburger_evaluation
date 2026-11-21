// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryRouter, createRoutesFromElements, RouterProvider, Route } from "react-router-dom";
import "../../../lib/i18n";
import { byText, cleanup, click, mount, need, type as typeInto } from "../../../test/dom";
import { ApiError } from "../../../api/client/buildApiClient";
import { todayDateOnly } from "../../../lib/date";
import ReviewNewPage from "./ReviewNewPage";

// R6: 評価は開いた直後は未選択で、選ばずに送信すると API を呼ばず、評価の近くにエラーを出し、評価の入力へ
// フォーカスを移す(送信のボタンは無効にしない)。選べば、従来どおり投稿できることも確かめる。
const create = vi.fn();
const shopState = vi.hoisted(() => ({
  shop: { id: "9", name: "Test Shop" } as { id: string; name: string; canReview?: boolean } | undefined,
  isLoading: false,
  error: undefined as unknown,
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1" }, isLoading: false }) }));
vi.mock("../../shops/hooks/useShops", () => ({
  useShopDetail: () => ({ data: shopState.shop, isLoading: shopState.isLoading, error: shopState.error }),
}));
vi.mock("../hooks/useReviewMutations", () => ({ useCreateReview: () => ({ create }) }));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
vi.mock("../../../api/meta", () => ({
  useMeta: () => ({ data: { text: { burgerNameMaxChars: 100, reviewCommentMaxChars: 2000 }, photo: { maxEdge: 1600, maxBytes: 5242880 } } }),
}));

const show = () => mount(<RouterProvider router={createMemoryRouter(createRoutesFromElements(<>
  <Route path="/reviews/new" element={<ReviewNewPage />} />
  <Route path="/discover" element={<h1>Destination</h1>} />
  <Route path="/reviews/:id" element={<h1>Saved review</h1>} />
</>), { initialEntries: ["/reviews/new?shop_id=9"] })} />);

beforeEach(() => {
  shopState.shop = { id: "9", name: "Test Shop" };
  shopState.isLoading = false;
  shopState.error = undefined;
  create.mockReset();
  create.mockResolvedValue({ id: "77" });
});
afterEach(async () => { vi.restoreAllMocks(); await cleanup(); });

describe("ReviewNewPage の評価(R6)", () => {
  it("開いた直後は、どの数字も選ばれていない(–)", async () => {
    const page = await show();
    expect(page.textContent).toContain("–");
    expect(page.querySelectorAll('[role="radio"][aria-checked="true"]').length).toBe(0);
  });

  it("選ばずに送信すると、API を呼ばず、評価の近くにエラーを出し、評価の入力にフォーカスが移る(ボタンは有効のまま)", async () => {
    const page = await show();
    const submit = need(byText<HTMLButtonElement>(page, "button", "Post review"), "Post review");
    expect(submit.disabled).toBe(false);

    await click(submit);

    expect(create).not.toHaveBeenCalled();
    expect(page.textContent).toContain("Please choose a rating before posting your review.");
    expect(submit.disabled).toBe(false);
    const active = page.querySelector('[role="radio"]');
    expect(document.activeElement).toBe(active);
  });

  it("選んで送信すれば、従来どおり投稿できる", async () => {
    const page = await show();
    await typeInto(need(page.querySelector<HTMLInputElement>("#burgerName"), "#burgerName"), "Cheeseburger");
    await click(need(byText<HTMLButtonElement>(page, "button", "3"), "rating 3"));
    await click(need(byText<HTMLButtonElement>(page, "button", "Post review"), "Post review"));

    expect(create).toHaveBeenCalledTimes(1);
    expect(create.mock.calls[0][0]).toMatchObject({ rating: 3, burgerName: "Cheeseburger", shopId: "9" });
  });

  it("実食日(visitedAt)に触れずに送信しても、従来どおり投稿できる(既定は空文字)", async () => {
    const page = await show();
    await click(need(byText<HTMLButtonElement>(page, "button", "3"), "rating 3"));
    await click(need(byText<HTMLButtonElement>(page, "button", "Post review"), "Post review"));

    expect(create).toHaveBeenCalledTimes(1);
    expect(create.mock.calls[0][0]).toMatchObject({ visitedAt: "" });
  });
});

describe("ReviewNewPage の実食日(visitedAt)", () => {
  it("today を上限にした日付の入力欄を出す", async () => {
    const page = await show();
    const input = need(page.querySelector<HTMLInputElement>("#visitedAt"), "#visitedAt");
    expect(input.type).toBe("date");
    expect(input.max).toBe(todayDateOnly());
    expect(page.querySelector('label[for="visitedAt"]')?.textContent).toContain("Visited on");
  });

  it("入力すると、送信時に visitedAt として渡る", async () => {
    const page = await show();
    await typeInto(need(page.querySelector<HTMLInputElement>("#burgerName"), "#burgerName"), "Cheeseburger");
    await typeInto(need(page.querySelector<HTMLInputElement>("#visitedAt"), "#visitedAt"), "2026-09-10");
    await click(need(byText<HTMLButtonElement>(page, "button", "3"), "rating 3"));
    await click(need(byText<HTMLButtonElement>(page, "button", "Post review"), "Post review"));

    expect(create).toHaveBeenCalledTimes(1);
    expect(create.mock.calls[0][0]).toMatchObject({ visitedAt: "2026-09-10" });
  });
});

it("評価だけ変更した場合も離脱を確認し、キャンセルすると入力を保つ", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  const page = await show();
  await click(need(byText<HTMLButtonElement>(page, "button", "2"), "評価2"));
  await click(need(page.querySelector('a[href="/discover"]'), "探すリンク"));
  expect(confirm).toHaveBeenCalledOnce();
  expect(page.querySelector('[role="radio"][aria-checked="true"]')?.textContent).toContain("2");
  const unload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
  confirm.mockReturnValue(true);
  await click(need(page.querySelector('a[href="/discover"]'), "探すリンク"));
  expect(page.querySelector("h1")?.textContent).toBe("Destination");
});

describe("ReviewNewPage の対象店舗の状態ガード", () => {
  it("店舗が canReview: false(閉業や却下など)のときは、フォームを出さず投稿不可の案内を出す", async () => {
    shopState.shop = { id: "9", name: "Closed Shop", canReview: false };
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("You can't review this shop");
  });

  it("存在しない店舗 ID(404)で取得が失敗したときは、生の ID と空フォームを出さず 404 画面を出す", async () => {
    shopState.shop = undefined;
    shopState.error = new ApiError(["Shop not found"], 404);
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.textContent).toContain("Sold out");
  });

  it("店舗取得が通信エラーで失敗したときは、404 売り切れ画面ではなく取得失敗のエラーを出す", async () => {
    shopState.shop = undefined;
    shopState.error = new Error("Network Error");
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Failed to load shops.");
    expect(page.textContent).not.toContain("Sold out");
  });
});
