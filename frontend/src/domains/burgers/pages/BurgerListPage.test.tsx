// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import "../../../lib/i18n";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import BurgerListPage from "./BurgerListPage";
import type { BurgerRanking } from "../api/types";

// 0 件のときの空の画面、カードの描画数(順位バッジを出さないこと)、「もっと見る」だけを確かめる。
// 一覧の取得そのものは useBurgers.test.ts が確かめる。
const state = vi.hoisted(() => ({
  burgers: [] as BurgerRanking[],
  hasNextPage: false,
  fetchNextPage: vi.fn(),
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: null, isLoading: false }) }));
vi.mock("../hooks/useBurgers", () => ({
  useBurgers: () => ({
    data: state.burgers,
    isLoading: false,
    error: undefined,
    hasNextPage: state.hasNextPage,
    fetchNextPage: state.fetchNextPage,
    isFetchingNextPage: false,
  }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ max: 5 }) }));

const burger = (over: Partial<BurgerRanking> = {}): BurgerRanking => ({
  id: "42",
  name: "テリヤキバーガー",
  photoUrl: null,
  shop: { id: "7", name: "バーガーラボ 中目黒" },
  averageRating: 4.8,
  weightedScore: 4.7,
  reviewCount: 8,
  ...over,
});

const show = () => mount(<MemoryRouter initialEntries={["/burgers"]}><BurgerListPage /></MemoryRouter>);

beforeEach(() => {
  state.burgers = [];
  state.hasNextPage = false;
  state.fetchNextPage = vi.fn();
});
afterEach(cleanup);

describe("BurgerListPage の空の一覧", () => {
  it("0 件のときは、遊びの文言の画面を出す", async () => {
    const page = await show();
    expect(page.textContent).toContain("Nobody's eaten here yet");
  });
});

describe("BurgerListPage のカード", () => {
  it("件数ぶんのカードを描画し、順位バッジは出さない", async () => {
    state.burgers = [burger({ id: "1" }), burger({ id: "2" }), burger({ id: "3" })];
    const page = await show();
    expect(page.querySelectorAll("article").length).toBe(3);
    expect(page.querySelector('[aria-label^="Rank"]')).toBeNull();
  });
});

describe("BurgerListPage の「もっと見る」", () => {
  it("次のページがあるときだけボタンを出し、押すと fetchNextPage を呼ぶ", async () => {
    state.hasNextPage = true;
    const page = await show();
    const button = need(byText<HTMLButtonElement>(page, "button", "Load more"), "Load more");
    await click(button);
    expect(state.fetchNextPage).toHaveBeenCalledTimes(1);
  });
});
