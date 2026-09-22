// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import type { ReviewDetailView } from "../api/types";
import ReviewEditPage from "./ReviewEditPage";

// can_edit による、フォームと「編集できません」の出し分けだけを確かめる。
const state = vi.hoisted(() => ({ review: undefined as ReviewDetailView | undefined }));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1" }, isLoading: false }) }));
vi.mock("../hooks/useReview", () => ({ useReview: () => ({ data: state.review, isLoading: false }) }));
vi.mock("../hooks/useReviewMutations", () => ({ useUpdateReview: () => ({ update: vi.fn() }) }));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { text: { reviewCommentMaxChars: 2000 }, photo: { maxEdge: 1600, maxBytes: 5242880 } } }) }));

const baseReview: ReviewDetailView = {
  id: "7",
  rating: 4,
  comment: "Tasty",
  photoUrl: null,
  createdAt: "2026-09-21T00:00:00Z",
  user: { id: "1", username: "alice" },
  burger: { id: "3", name: "Cheeseburger", averageRating: 4.2, reviewCount: 5, weightedScore: 4.1, confidence: 0.8 },
  canEdit: true,
  canReview: false,
  shop: { id: "9", name: "Test Shop" },
};

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/reviews/7/edit"]}>
      <Routes>
        <Route path="/reviews/:id/edit" element={<ReviewEditPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.review = baseReview;
});
afterEach(cleanup);

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
