// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import type { BurgerDetail } from "../api/types";
import type { ReviewView } from "../../reviews/api/types";
import BurgerDetailPage from "./BurgerDetailPage";

// バーガー名・お店へのリンク・レビューの並びと、0 件・見つからないときの画面だけを確かめる。
const state = vi.hoisted(() => ({
  authUser: null as { id: string } | null,
  burger: undefined as BurgerDetail | undefined,
  error: undefined as unknown,
  reviews: [] as ReviewView[],
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
vi.mock("../hooks/useBurger", () => ({ useBurger: () => ({ data: state.burger, isLoading: false, error: state.error }) }));
vi.mock("../../reviews/hooks/useReviews", () => ({
  useReviews: () => ({
    data: state.reviews,
    isLoading: false,
    error: undefined,
    hasNextPage: false,
    fetchNextPage: vi.fn(),
    isFetchingNextPage: false,
  }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const baseBurger: BurgerDetail = {
  id: "3",
  name: "Cheeseburger",
  shops: [
    { id: "9", name: "Test Shop" },
    { id: "10", name: "Second Shop" },
  ],
  averageRating: 4.2,
  weightedScore: 4.1,
  reviewCount: 2,
};

function review(id: string): ReviewView {
  return {
    id,
    rating: 5,
    comment: "Tasty",
    photoUrl: null,
    createdAt: "2026-09-21T00:00:00Z",
    user: { id: "2", username: "alice" },
    burger: { id: "3", name: "Cheeseburger", averageRating: 4.2, reviewCount: 2, weightedScore: 4.1, confidence: 0.8 },
    canEdit: false,
  };
}

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/burgers/3"]}>
      <Routes>
        <Route path="/burgers/:id" element={<BurgerDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.authUser = null;
  state.burger = baseBurger;
  state.error = undefined;
  state.reviews = [review("1"), review("2")];
});
afterEach(cleanup);

describe("BurgerDetailPage の状態の表示", () => {
  it("バーガー名・お店へのリンク・平均・件数を出す", async () => {
    const page = await show();

    expect(page.querySelector("h1")?.textContent).toBe("Cheeseburger");
    expect(page.querySelector('a[href="/shops/9"]')?.textContent).toBe("Test Shop");
    expect(page.querySelector('a[href="/shops/10"]')?.textContent).toBe("Second Shop");
    expect(page.textContent).toContain("4.2");
    expect(page.textContent).toContain("2 reviews");
  });

  it("レビューは、取得した順のまま並ぶ(並び替えは backend・hook の責務)", async () => {
    const page = await show();

    const links = [...page.querySelectorAll('a[href^="/reviews/"]')].map((a) => a.getAttribute("href"));
    expect(links).toEqual(["/reviews/1", "/reviews/2"]);
  });

  it("見つからない(404)ときは、売り切れの画面を出す", async () => {
    state.burger = undefined;
    state.error = new Error("not found");
    const page = await show();

    expect(page.textContent).toContain("404");
    expect(page.textContent).toContain("Sold out");
  });

  it("レビューが 1 件もないとき(reviewCount が null)は、空の画面を出し、平均は出さない", async () => {
    state.burger = { ...baseBurger, averageRating: null, weightedScore: null, reviewCount: null };
    state.reviews = [];
    const page = await show();

    expect(page.textContent).toContain("Nobody's eaten here yet");
    expect(page.textContent).not.toContain("4.2");
  });

  it("投稿直後で reviewCount が統計の再計算待ち(null)でも、既に取得できているレビューは出す", async () => {
    // 初めてのレビューを投稿した直後: 統計の再計算(非同期)がまだ reviewCount に反映されていない一方、
    // レビュー一覧(同期)は既にそのレビューを含んでいる状態を再現する。
    state.burger = { ...baseBurger, averageRating: null, weightedScore: null, reviewCount: null };
    state.reviews = [review("1")];
    const page = await show();

    const links = [...page.querySelectorAll('a[href^="/reviews/"]')].map((a) => a.getAttribute("href"));
    expect(links).toEqual(["/reviews/1"]);
    expect(page.textContent).not.toContain("Nobody's eaten here yet");
  });

  it("最後のレビューを削除した直後で reviewCount が古い値のままでも、空の画面を出す", async () => {
    // 最後のレビューを削除した直後: レビュー一覧(同期)は既に空になっている一方、
    // reviewCount・averageRating(統計の再計算(非同期))はまだ削除前の値のままの状態を再現する。
    state.reviews = [];
    const page = await show();

    expect(page.textContent).toContain("4.2");
    expect(page.textContent).toContain("Nobody's eaten here yet");
  });
});
