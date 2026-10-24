// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import i18n from "../../../lib/i18n";
import { cleanup, mount, need } from "../../../test/dom";
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
vi.mock("../../../api/meta", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../api/meta")>()),
  useMeta: () => ({ data: { prefectures: [{ code: 13, nameJa: "東京都", nameEn: "Tokyo" }] } }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const baseShop: ShopDetail = {
  id: "7",
  name: "Shake Shack",
  status: "active",
  photoUrl: null,
  averageRating: null,
  reviewCount: 0,
  mapUrl: null,
  prefectureCode: null,
  city: "",
  streetAddress: "",
  closedAt: null,
  moderationNote: null,
  creator: null,
  reviews: [],
  canReview: false,
};

const show = (path = "/shops/7") =>
  mount(
    <MemoryRouter initialEntries={[path]}>
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
afterEach(async () => {
  await cleanup();
  await i18n.changeLanguage("en");
});

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

  it("closedAt があるときは「Closed」の札を出す(status の札とは独立)", async () => {
    state.shop = { ...baseShop, closedAt: "2026-01-01T00:00:00Z" };
    const page = await show();
    expect(page.textContent).toContain("Closed");
  });

  it("closedAt が null のときは「Closed」の札を出さない", async () => {
    const page = await show();
    expect(page.textContent).not.toContain("Closed");
  });
});

describe("ShopDetailPage の地図リンク(map_url)", () => {
  it("map_url があるときは、新しいタブで安全に開くリンクを出す", async () => {
    state.shop = { ...baseShop, mapUrl: "https://maps.example.com/shop" };
    const page = await show();
    const link = page.querySelector<HTMLAnchorElement>('a[href="https://maps.example.com/shop"]');
    expect(link).not.toBeNull();
    expect(link?.textContent).toBe("View on map");
    expect(link?.target).toBe("_blank");
    expect(link?.rel).toBe("noopener noreferrer");
  });

  it("map_url が null のときは、リンクを出さない", async () => {
    state.shop = { ...baseShop, mapUrl: null };
    const page = await show();
    expect(page.textContent).not.toContain("View on map");
  });
});

describe("ShopDetailPage の住所", () => {
  const withAddress = { ...baseShop, prefectureCode: 13, city: "渋谷区", streetAddress: "神南1-2-3" };

  it("日本語表示では「都道府県名 市区町村 番地以降」を半角スペースでつないで出す", async () => {
    await i18n.changeLanguage("ja");
    state.shop = withAddress;
    const page = await show();
    expect(page.textContent).toContain("東京都 渋谷区 神南1-2-3");
  });

  it("英語表示では、都道府県を英語名で出す", async () => {
    state.shop = withAddress;
    const page = await show();
    expect(page.textContent).toContain("Tokyo 渋谷区 神南1-2-3");
  });

  it("空の部分は飛ばしてつなぐ(都道府県が未設定なら市区町村から始まる)", async () => {
    state.shop = { ...withAddress, prefectureCode: null, streetAddress: "" };
    const page = await show();
    const name = need(page.querySelector("h1"), "店名");
    expect(name.nextElementSibling?.textContent).toBe("渋谷区");
  });

  it("住所が全部未設定なら、住所の行を出さない", async () => {
    const page = await show();
    const name = need(page.querySelector("h1"), "店名");
    expect(name.nextElementSibling).toBeNull();
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

it("記録入口から選んだ店舗では、投稿可否を反映しつつ記録入口への戻り先を保つ", async () => {
  state.authUser = { id: "1" };
  state.shop = { ...baseShop, canReview: false, status: "pending" };
  const page = await show("/shops/7?from=record");
  expect(page.querySelector('a[href="/record"]')).not.toBeNull();
  expect(page.querySelector('a[href^="/reviews/new"]')).toBeNull();
});
