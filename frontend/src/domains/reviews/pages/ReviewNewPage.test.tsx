// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { byText, cleanup, click, mount, need, type as typeInto } from "../../../test/dom";
import ReviewNewPage from "./ReviewNewPage";

// R6: 評価は開いた直後は未選択で、選ばずに送信すると API を呼ばず、評価の近くにエラーを出し、評価の入力へ
// フォーカスを移す(送信のボタンは無効にしない)。選べば、従来どおり投稿できることも確かめる。
const create = vi.fn();
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1" }, isLoading: false }) }));
vi.mock("../../shops/hooks/useShops", () => ({ useShopDetail: () => ({ data: { id: "9", name: "Test Shop" } }) }));
vi.mock("../hooks/useReviewMutations", () => ({ useCreateReview: () => ({ create }) }));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
vi.mock("../../../api/meta", () => ({
  useMeta: () => ({ data: { text: { burgerNameMaxChars: 100, reviewCommentMaxChars: 2000 }, photo: { maxEdge: 1600, maxBytes: 5242880 } } }),
}));

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/reviews/new?shop_id=9"]}>
      <Routes>
        <Route path="/reviews/new" element={<ReviewNewPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  create.mockReset();
  create.mockResolvedValue({ id: "77" });
});
afterEach(cleanup);

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
});
