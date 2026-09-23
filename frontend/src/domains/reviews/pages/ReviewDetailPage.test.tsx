// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import type { ReviewDetailView } from "../api/types";
import ReviewDetailPage from "./ReviewDetailPage";

// can_edit による「編集」「削除」の出し分けと、見つからないときの画面だけを確かめる。
const state = vi.hoisted(() => ({
  authUser: null as { id: string } | null,
  review: undefined as ReviewDetailView | undefined,
  error: undefined as unknown,
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
vi.mock("../hooks/useReview", () => ({ useReview: () => ({ data: state.review, isLoading: false, error: state.error }) }));
vi.mock("../hooks/useReviewMutations", () => ({ useDeleteReview: () => ({ destroy: vi.fn() }) }));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const baseReview: ReviewDetailView = {
  id: "7",
  rating: 4,
  comment: "Tasty",
  photoUrl: null,
  createdAt: "2026-09-21T00:00:00Z",
  user: { id: "2", username: "alice" },
  burger: { id: "3", name: "Cheeseburger", averageRating: 4.2, reviewCount: 5, weightedScore: 4.1, confidence: 0.8 },
  canEdit: false,
  canReview: false,
  shop: { id: "9", name: "Test Shop" },
};

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/reviews/7"]}>
      <Routes>
        <Route path="/reviews/:id" element={<ReviewDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.authUser = null;
  state.review = baseReview;
  state.error = undefined;
});
afterEach(cleanup);

describe("ReviewDetailPage の状態の表示", () => {
  it("見つからない(404)ときは、売り切れの画面を出す", async () => {
    state.review = undefined;
    state.error = new Error("not found");
    const page = await show();
    expect(page.textContent).toContain("404");
    expect(page.textContent).toContain("Sold out");
  });

  it("バーガー名・ショップ名・バーガーの平均を出す", async () => {
    const page = await show();
    expect(page.querySelector("h1")?.textContent).toBe("Cheeseburger");
    expect(page.textContent).toContain("Test Shop");
    expect(page.textContent).toContain("4.2");
    expect(page.textContent).toContain("5 reviews");
  });

  it("バーガー名の見出しは、バーガー詳細へのリンクになる", async () => {
    const page = await show();
    const link = page.querySelector("h1 a");
    expect(link?.textContent).toBe("Cheeseburger");
    expect(link?.getAttribute("href")).toBe("/burgers/3");
  });
});

describe("ReviewDetailPage の「編集」「削除」(can_edit)", () => {
  it("can_edit が true のときだけ出る", async () => {
    state.review = { ...baseReview, canEdit: true };
    const page = await show();
    expect(page.textContent).toContain("Edit");
    expect(page.textContent).toContain("Delete");
  });

  it("can_edit が false のときは出ない(他人のレビュー)", async () => {
    state.review = { ...baseReview, canEdit: false };
    const page = await show();
    expect(page.textContent).not.toContain("Edit");
    expect(page.textContent).not.toContain("Delete");
  });
});
