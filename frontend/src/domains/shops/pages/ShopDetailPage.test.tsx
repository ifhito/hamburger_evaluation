// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import type { ShopDetail } from "../api/types";
import ShopDetailPage from "./ShopDetailPage";

// can_review・status に応じた出し分け(デザインの案内・「レビューを書く」)と、0 件・見つからないときの画面だけを確かめる。
const state = vi.hoisted(() => ({
  authUser: null as { id: string } | null,
  shop: undefined as ShopDetail | undefined,
  error: undefined as unknown,
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
vi.mock("../hooks/useShops", () => ({
  useShopDetail: () => ({ data: state.shop, isLoading: false, error: state.error }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const baseShop: ShopDetail = {
  id: "7",
  name: "Shake Shack",
  status: "active",
  photoUrl: null,
  averageRating: null,
  reviewCount: 0,
  moderationNote: null,
  creator: null,
  reviews: [],
  canReview: false,
};

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/shops/7"]}>
      <Routes>
        <Route path="/shops/:id" element={<ShopDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.authUser = null;
  state.shop = baseShop;
  state.error = undefined;
});
afterEach(cleanup);

describe("ShopDetailPage の状態の表示", () => {
  it("見つからない(404)ときは、売り切れの画面を出す", async () => {
    state.shop = undefined;
    state.error = new Error("not found");
    const page = await show();
    expect(page.textContent).toContain("404");
    expect(page.textContent).toContain("Sold out");
  });

  it("審査待ちのときは、札と案内を出す", async () => {
    state.shop = { ...baseShop, status: "pending" };
    const page = await show();
    expect(page.textContent).toContain("Pending review");
    expect(page.textContent).toContain("This shop is awaiting review and is not public yet.");
  });

  it("却下されたときは、札と理由を出し、レビューの案内は出ない", async () => {
    state.shop = { ...baseShop, status: "rejected", moderationNote: "Not a real shop" };
    const page = await show();
    expect(page.textContent).toContain("Rejected");
    expect(page.textContent).toContain("Reason: Not a real shop");
    expect(page.textContent).not.toContain("Write a review");
  });

  it("レビューが 0 件のときは、遊びの文言の画面を出す", async () => {
    const page = await show();
    expect(page.textContent).toContain("Nobody's eaten here yet");
  });
});

describe("ShopDetailPage の「レビューを書く」(can_review)", () => {
  it("can_review が true のときだけ出る", async () => {
    state.shop = { ...baseShop, canReview: true };
    const page = await show();
    expect(page.textContent).toContain("Write a review");
  });

  it("can_review が false のときは出ない", async () => {
    state.shop = { ...baseShop, canReview: false };
    const page = await show();
    expect(page.textContent).not.toContain("Write a review");
  });

  it("サインインしていない(can_review が false)ときは、サインインへの案内を出す", async () => {
    state.authUser = null;
    state.shop = { ...baseShop, canReview: false };
    const page = await show();
    expect(page.textContent).toContain("Sign in to write a review");
  });
});
