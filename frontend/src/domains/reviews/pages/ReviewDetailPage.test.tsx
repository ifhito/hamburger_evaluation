// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import type { ReviewDetailView } from "../api/types";
import ReviewDetailPage from "./ReviewDetailPage";

// can_edit による「編集」「削除」の出し分けと、見つからないときの画面だけを確かめる。
const destroy = vi.hoisted(() => vi.fn());
const state = vi.hoisted(() => ({
  authUser: null as { id: string } | null,
  review: undefined as ReviewDetailView | undefined,
  error: undefined as unknown,
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
vi.mock("../hooks/useReview", () => ({ useReview: () => ({ data: state.review, isLoading: false, error: state.error }) }));
vi.mock("../hooks/useReviewMutations", () => ({ useDeleteReview: () => ({ destroy }) }));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const baseReview: ReviewDetailView = {
  id: "7",
  rating: 4,
  comment: "Tasty",
  photoUrl: null,
  createdAt: "2026-09-21T00:00:00Z",
  visitedAt: null,
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
  destroy.mockReset();
});
afterEach(cleanup);

describe("ReviewDetailPage の状態の表示", () => {
  it("見つからない(404)ときは、売り切れの画面を出す", async () => {
    state.review = undefined;
    state.error = new ApiError(["Not found"], 404);
    const page = await show();
    expect(page.textContent).toContain("404");
    expect(page.textContent).toContain("Sold out");
  });

  it("通信失敗(ApiError 以外の例外)では、404 売り切れ画面ではなく取得失敗のエラーを出す", async () => {
    state.review = undefined;
    state.error = new Error("Network Error");
    const page = await show();
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Failed to load reviews.");
    expect(page.textContent).not.toContain("Sold out");
  });

  it("バーガー名・ショップ名を出す", async () => {
    const page = await show();
    expect(page.querySelector("h1")?.textContent).toBe("Cheeseburger");
    expect(page.textContent).toContain("Test Shop");
    expect(page.textContent).not.toContain("4.2");
    expect(page.textContent).not.toContain("5 reviews");
  });

  it("バーガー名の見出しは、バーガー詳細へのリンクになる", async () => {
    const page = await show();
    const link = page.querySelector("h1 a");
    expect(link?.textContent).toBe("Cheeseburger");
    expect(link?.getAttribute("href")).toBe("/burgers/3");
  });

  it("comment が空のときは、空のコメント欄を出さない(評価だけのレビュー)", async () => {
    state.review = { ...baseReview, comment: "" };
    const page = await show();
    expect(page.querySelector('[class*="comment"]')).toBeNull();
  });
});

describe("ReviewDetailPage の実食日(visitedAt)", () => {
  it("visitedAt があるときは、実食日を出す", async () => {
    state.review = { ...baseReview, visitedAt: "2026-09-10" };
    const page = await show();
    expect(page.textContent).toContain("Visited Sep 10, 2026");
  });

  it("visitedAt が null のときは、実食日を出さない", async () => {
    const page = await show();
    expect(page.textContent).not.toContain("Visited ");
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

  it("削除が API エラーで失敗したときは、未処理例外にせず画面にエラーを表示する", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    destroy.mockRejectedValueOnce(new ApiError(["Forbidden"], 403));
    state.review = { ...baseReview, canEdit: true };
    try {
      const page = await show();
      await click(need(byText(page, "button", "Delete"), "Delete"));
      expect(page.querySelector('[role="alert"]')?.textContent).toContain("Could not delete the review");
      expect(page.querySelector('[role="alert"]')?.textContent).toContain("Forbidden");
    } finally {
      confirmSpy.mockRestore();
    }
  });

  it("取得が 503 で失敗したときは、404 売り切れ画面ではなく取得失敗のエラーを出す", async () => {
    state.review = undefined;
    state.error = new ApiError(["Service unavailable"], 503);
    const page = await show();
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Failed to load reviews.");
    expect(page.textContent).not.toContain("Sold out");
  });
});

describe("ReviewDetailPage の共有導線", () => {
  it("通常のレビュー詳細画面では、本文下に X 投稿リンクとリンクコピーボタンを出す", async () => {
    const page = await show();
    expect(page.textContent).not.toContain("Your review is live!");
    const xLinks = page.querySelectorAll<HTMLAnchorElement>('a[href^="https://x.com/intent/post?"]');
    expect(xLinks.length).toBe(1);
    expect(byText(page, "button", "Copy link")).toBeTruthy();
  });

  it("投稿直後(state.justPosted)に遷移してきたときは投稿完了バナーを出し、閉じると通常の共有欄へ切り替わる", async () => {
    const page = await mount(
      <MemoryRouter initialEntries={[{ pathname: "/reviews/7", state: { justPosted: true } }]}>
        <Routes>
          <Route path="/reviews/:id" element={<ReviewDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(page.textContent).toContain("Your review is live!");
    // バナー表示中は共有ボタンが二重に出ないこと
    expect(page.querySelectorAll('a[href^="https://x.com/intent/post?"]').length).toBe(1);

    await click(need(byText<HTMLButtonElement>(page, "button", "Dismiss"), "Dismiss"));
    expect(page.textContent).not.toContain("Your review is live!");
    expect(page.querySelectorAll('a[href^="https://x.com/intent/post?"]').length).toBe(1);
  });
});
