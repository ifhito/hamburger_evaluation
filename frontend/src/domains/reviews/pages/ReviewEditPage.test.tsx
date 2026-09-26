// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryRouter, createRoutesFromElements, RouterProvider, Route } from "react-router-dom";
import "../../../lib/i18n";
import { byText, click, cleanup, mount, need, type as typeInto } from "../../../test/dom";
import type { ReviewDetailView } from "../api/types";
import ReviewEditPage from "./ReviewEditPage";

// can_edit による、フォームと「編集できません」の出し分けと、見つからない・取得に失敗したときの画面を確かめる。
const state = vi.hoisted(() => ({ review: undefined as ReviewDetailView | undefined, error: undefined as unknown }));
const update = vi.fn();
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1" }, isLoading: false }) }));
vi.mock("../hooks/useReview", () => ({ useReview: () => ({ data: state.review, isLoading: false, error: state.error }) }));
vi.mock("../hooks/useReviewMutations", () => ({ useUpdateReview: () => ({ update }) }));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { text: { reviewCommentMaxChars: 2000 }, photo: { maxEdge: 1600, maxBytes: 5242880 } } }) }));

const baseReview: ReviewDetailView = {
  id: "7",
  rating: 4,
  comment: "Tasty",
  photoUrl: null,
  createdAt: "2026-09-21T00:00:00Z",
  visitedAt: null,
  user: { id: "1", username: "alice" },
  burger: { id: "3", name: "Cheeseburger", averageRating: 4.2, reviewCount: 5, weightedScore: 4.1, confidence: 0.8 },
  canEdit: true,
  canReview: false,
  shop: { id: "9", name: "Test Shop" },
};

const show = () => mount(<RouterProvider router={createMemoryRouter(createRoutesFromElements(<>
  <Route path="/reviews/:id/edit" element={<ReviewEditPage />} />
  <Route path="/discover" element={<h1>Destination</h1>} />
  <Route path="/reviews/:id" element={<h1>Saved review</h1>} />
</>), { initialEntries: ["/reviews/7/edit"] })} />);

beforeEach(() => {
  state.review = baseReview;
  state.error = undefined;
  update.mockReset();
  update.mockResolvedValue({ id: "7" });
});
afterEach(async () => { vi.restoreAllMocks(); await cleanup(); });

describe("ReviewEditPage の、見つからない・取得に失敗したとき", () => {
  it("売り切れの画面を出す(フォームも「編集できません」も出さない)", async () => {
    state.review = undefined;
    state.error = new Error("not found");
    const page = await show();
    expect(page.textContent).toContain("404");
    expect(page.textContent).toContain("Sold out");
    expect(page.querySelector("form")).toBeNull();
  });
});

describe("ReviewEditPage の can_edit", () => {
  it("can_edit が true のときは、既存の評価を選んだフォームを出す", async () => {
    const page = await show();
    const active = page.querySelector('[role="radio"][aria-checked="true"]');
    expect(active?.textContent).toBe("4");
    expect(page.querySelector("form")).not.toBeNull();
  });

  it("can_edit が false のときは、フォームを出さず、編集できない旨を出す", async () => {
    state.review = { ...baseReview, canEdit: false };
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.textContent).toContain("You can't edit this review");
  });
});

describe("ReviewEditPage の実食日(visitedAt)", () => {
  it("review.visitedAt があるときは、その値をフォームに描画する", async () => {
    state.review = { ...baseReview, visitedAt: "2026-09-10" };
    const page = await show();
    const input = need(page.querySelector<HTMLInputElement>("#visitedAt"), "#visitedAt");
    expect(input.value).toBe("2026-09-10");
  });

  it("review.visitedAt が null のときは、空で初期化する", async () => {
    const page = await show();
    const input = need(page.querySelector<HTMLInputElement>("#visitedAt"), "#visitedAt");
    expect(input.value).toBe("");
  });

  it("変えて送信すると、新しい値が update に渡る", async () => {
    const page = await show();
    const input = need(page.querySelector<HTMLInputElement>("#visitedAt"), "#visitedAt");
    await typeInto(input, "2026-09-15");
    await click(need(byText<HTMLButtonElement>(page, "button", "Save changes"), "Save changes"));

    expect(update).toHaveBeenCalledTimes(1);
    expect(update.mock.calls[0][0]).toMatchObject({ visitedAt: "2026-09-15" });
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
