// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import "../../../lib/i18n";
import { byText, cleanup, mount, need } from "../../../test/dom";
import ReviewListPage from "./ReviewListPage";

// 「レビューを書く」の出し分け(サインインの状態)と、0 件のときの空の画面だけを確かめる。
// 一覧の取得そのものは useReviews.test.ts が確かめる。
const state = vi.hoisted(() => ({
  authUser: null as { id: string } | null,
  reviews: [] as { id: string; rating: number; comment: string; photoUrl: string | null; createdAt: string; visitedAt: string | null; user: { id: string; username: string } | null; burger: null }[],
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
vi.mock("../hooks/useReviews", () => ({
  useReviews: () => ({ data: state.reviews, isLoading: false, error: undefined, hasNextPage: false, fetchNextPage: vi.fn(), isFetchingNextPage: false }),
}));
vi.mock("../hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const show = () => mount(<MemoryRouter initialEntries={["/reviews"]}><ReviewListPage /></MemoryRouter>);

beforeEach(() => {
  state.authUser = null;
  state.reviews = [{ id: "1", rating: 5, comment: "Great", photoUrl: null, createdAt: "2026-09-21T00:00:00Z", visitedAt: null, user: { id: "2", username: "alice" }, burger: null }];
});
afterEach(cleanup);

describe("ReviewListPage の「レビューを書く」", () => {
  it("サインインしていないときは出ない", async () => {
    const page = await show();
    expect(page.textContent).not.toContain("Write a review");
  });

  it("サインインしているときは、ショップ一覧へのリンクとして出る", async () => {
    state.authUser = { id: "1" };
    const page = await show();
    const link = need(byText<HTMLAnchorElement>(page, "a", "Write a review"), "Write a review");
    expect(link.getAttribute("href")).toBe("/shops");
  });
});

describe("ReviewListPage の空の一覧", () => {
  it("0 件のときは、遊びの文言の画面を出す", async () => {
    state.reviews = [];
    const page = await show();
    expect(page.textContent).toContain("Nobody's eaten here yet");
  });
});
