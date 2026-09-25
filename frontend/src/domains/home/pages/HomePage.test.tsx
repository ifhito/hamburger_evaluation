// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import "../../../lib/i18n";
import { click, cleanup, mount, type } from "../../../test/dom";
import HomePage from "./HomePage";
import type { BurgerRanking } from "../../burgers/api/types";
import type { ReviewView } from "../../reviews/api/types";
import type { Shop } from "../../shops/api/types";

// 各セクションのデータ取得は、対応する hook(useBurgerRanking・useReviews・useShops)に任せ、ここでは
// 返ってきたデータを、順番どおり・件数の上限どおりに描画すること、0 件のときに空の状態を出すこと、
// ヒーローの検索が /shops へ絞り込みつきで移動することだけを確かめる。
const state = vi.hoisted(() => {
  const s = {
    ranking: [] as BurgerRanking[],
    rankingLoading: false,
    rankingError: undefined as unknown,
    reviews: [] as ReviewView[],
    reviewsLoading: false,
    reviewsError: undefined as unknown,
    shops: [] as Shop[],
    shopsLoading: false,
    shopsError: undefined as unknown,
    // useShops へ渡された引数(sort など)を確かめるための spy。呼び出しごとの結果は state.shops 等から作る
    useShopsMock: vi.fn((params?: { keyword?: string; sort?: "newest" }) => {
      void params;
      return { data: s.shops, isLoading: s.shopsLoading, error: s.shopsError };
    }),
  };
  return s;
});

vi.mock("../../burgers/hooks/useBurgerRanking", () => ({
  useBurgerRanking: () => ({ data: state.ranking, isLoading: state.rankingLoading, error: state.rankingError }),
}));
vi.mock("../../reviews/hooks/useReviews", () => ({
  useReviews: () => ({ data: state.reviews, isLoading: state.reviewsLoading, error: state.reviewsError }),
}));
vi.mock("../../shops/hooks/useShops", () => ({
  useShops: (params?: { keyword?: string; sort?: "newest" }) => state.useShopsMock(params),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));
// Layout がヘッダーの出し分けに使う(このページ自身は認証状態を読まない)
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: null, isLoading: false }) }));

function burger(over: Partial<BurgerRanking>): BurgerRanking {
  return {
    id: "b1",
    name: "Burger",
    photoUrl: null,
    shop: { id: "s1", name: "Shop" },
    averageRating: 4,
    weightedScore: 4,
    reviewCount: 1,
    ...over,
  };
}

function review(over: Partial<ReviewView>): ReviewView {
  return {
    id: "r1",
    rating: 4,
    comment: "Tasty",
    photoUrl: null,
    createdAt: "2026-09-21T00:00:00Z",
    visitedAt: null,
    user: { id: "u1", username: "alice" },
    burger: { id: "b1", name: "Burger", averageRating: 4, reviewCount: 1, weightedScore: 4, confidence: 0.5 },
    canEdit: false,
    ...over,
  };
}

function shop(over: Partial<Shop>): Shop {
  return {
    id: "s1",
    name: "Shop",
    status: "active",
    photoUrl: null,
    averageRating: 4,
    reviewCount: 1,
    mapUrl: null,
    closedAt: null,
    ...over,
  };
}

// / から /shops への移動先(検索先)を確かめるための、着地点だけの目印。
function ShopsProbe() {
  const location = useLocation();
  return <div data-testid="shops-probe">{location.pathname + location.search}</div>;
}

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/shops" element={<ShopsProbe />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.ranking = [];
  state.rankingLoading = false;
  state.rankingError = undefined;
  state.reviews = [];
  state.reviewsLoading = false;
  state.reviewsError = undefined;
  state.shops = [];
  state.shopsLoading = false;
  state.shopsError = undefined;
  state.useShopsMock.mockClear();
});
afterEach(cleanup);

describe("HomePage のヒーロー", () => {
  it("見出しと検索欄を表示する", async () => {
    const page = await show();
    expect(page.textContent).toContain("Find your favorite burger");
    expect(page.querySelector("input[type='text']")).not.toBeNull();
  });

  it("店名を入力して探すと、絞り込みつきで /shops へ移動する", async () => {
    const page = await show();
    const input = page.querySelector<HTMLInputElement>("input[type='text']");
    if (!input) throw new Error("search input not found");
    await type(input, "Shake Shack");
    const button = [...page.querySelectorAll("button")].find((b) => b.textContent === "Search");
    if (!button) throw new Error("search button not found");
    await click(button);
    const probe = page.querySelector("[data-testid='shops-probe']");
    expect(probe?.textContent).toBe("/shops?keyword=Shake+Shack");
  });
});

describe("HomePage のバーガーランキング", () => {
  it("hook が返した順で、6 件までに切り詰めて表示する", async () => {
    state.ranking = Array.from({ length: 8 }, (_, i) => burger({ id: `b${i + 1}`, name: `Burger ${i + 1}` }));
    const page = await show();
    const grid = page.querySelector('[data-testid="ranking-grid"]');
    const names = [...(grid?.querySelectorAll('p a[href^="/burgers/"]') ?? [])].map((a) => a.textContent);
    expect(names).toEqual(["Burger 1", "Burger 2", "Burger 3", "Burger 4", "Burger 5", "Burger 6"]);
  });

  it("0 件のときは、ランキング専用の空の文言を出す", async () => {
    state.ranking = [];
    const page = await show();
    expect(page.textContent).toContain("No ranking yet");
  });

  it("取得中は読み込み中の表示を出し、ほかのセクションはそのまま表示する", async () => {
    state.rankingLoading = true;
    state.reviews = [review({ id: "r1", comment: "Comment 1" })];
    const page = await show();
    expect(page.textContent).toContain("Grilling…");
    expect(page.textContent).toContain("Comment 1");
  });

  it("失敗したら読み込みエラーの文言を出し、ほかのセクションはそのまま表示する", async () => {
    state.rankingError = new Error("boom");
    state.shops = [shop({ id: "s1", name: "Shop 1" })];
    const page = await show();
    expect(page.textContent).toContain("Failed to load the burger ranking.");
    expect(page.textContent).toContain("Shop 1");
  });
});

describe("HomePage の最新レビュー", () => {
  it("hook が返した順で、5 件までに切り詰めて表示する", async () => {
    state.reviews = Array.from({ length: 7 }, (_, i) => review({ id: `r${i + 1}`, comment: `Comment ${i + 1}` }));
    const page = await show();
    const grid = page.querySelector('[data-testid="reviews-grid"]');
    const comments = [...(grid?.querySelectorAll("p") ?? [])].map((p) => p.textContent).filter((t) => t?.startsWith("Comment "));
    expect(comments).toEqual(["Comment 1", "Comment 2", "Comment 3", "Comment 4", "Comment 5"]);
  });

  it("0 件のときは、既定の空の文言(レビュー一覧と共通)を出す", async () => {
    state.reviews = [];
    const page = await show();
    expect(page.textContent).toContain("Nobody's eaten here yet");
  });
});

describe("HomePage の新着ショップ", () => {
  it("hook が返した順で、6 件までに切り詰めて表示する", async () => {
    state.shops = Array.from({ length: 8 }, (_, i) => shop({ id: `s${i + 1}`, name: `Shop ${i + 1}` }));
    const page = await show();
    const grid = page.querySelector('[data-testid="shops-grid"]');
    const names = [...(grid?.querySelectorAll("h3") ?? [])].map((h) => h.textContent);
    expect(names).toEqual(["Shop 1", "Shop 2", "Shop 3", "Shop 4", "Shop 5", "Shop 6"]);
  });

  it("0 件のときは、新着ショップ専用の空の文言を出す", async () => {
    state.shops = [];
    const page = await show();
    expect(page.textContent).toContain("No shops yet");
  });

  it("useShops を newest 順で呼び出す(古い既定の並び順に戻らないことを確かめる)", async () => {
    await show();
    expect(state.useShopsMock).toHaveBeenCalledWith({ sort: "newest" });
  });
});
