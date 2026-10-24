// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import i18n from "../../../lib/i18n";
import { choose, cleanup, mount, need, type } from "../../../test/dom";
import ShopListPage from "./ShopListPage";
import RecordPage from "../../../app/navigation/RecordPage";

// 「ショップを追加」「ショップを管理」の出し分け(サインインの状態・can_moderate)と、0 件のときの空の画面だけを確かめる。
// 一覧の取得そのものは useShops.test.ts が確かめる。
const state = vi.hoisted(() => ({
  authUser: null as { id: string; canModerate: boolean } | null,
  shops: [] as { id: string; name: string; status: string; photoUrl: string | null; averageRating: number | null; reviewCount: number }[],
}));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: state.authUser, isLoading: false }) }));
const useShopsParams = vi.hoisted(() => vi.fn());
vi.mock("../hooks/useShops", () => ({
  useShops: (params: unknown) => (useShopsParams(params), { data: state.shops, isLoading: false, error: undefined, hasNextPage: false, fetchNextPage: vi.fn(), isFetchingNextPage: false }),
}));
vi.mock("../../../api/meta", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../api/meta")>()),
  useMeta: () => ({
    data: {
      prefectures: [
        { code: 1, nameJa: "北海道", nameEn: "Hokkaido" },
        { code: 13, nameJa: "東京都", nameEn: "Tokyo" },
      ],
    },
  }),
}));
vi.mock("../../reviews/hooks/useRatingRange", () => ({ useRatingRange: () => ({ min: 1, max: 5 }) }));

const show = (initialEntries: string[] = ["/shops"]) => mount(<MemoryRouter initialEntries={initialEntries}><ShopListPage /></MemoryRouter>);

beforeEach(() => {
  state.authUser = null;
  state.shops = [{ id: "7", name: "Shake Shack", status: "active", photoUrl: null, averageRating: 4.5, reviewCount: 2 }];
});
afterEach(async () => {
  await cleanup();
  await i18n.changeLanguage("en");
});

describe("ShopListPage のツールバー", () => {
  it("サインインしていないときは、どちらのボタンも出ない", async () => {
    const page = await show();
    expect(page.textContent).not.toContain("Add a shop");
    expect(page.textContent).not.toContain("Moderate shops");
  });

  it("サインインしているときは「ショップを追加」だけ出る(can_moderate が false)", async () => {
    state.authUser = { id: "1", canModerate: false };
    const page = await show();
    expect(page.textContent).toContain("Add a shop");
    expect(page.textContent).not.toContain("Moderate shops");
  });

  it("can_moderate なら「ショップを管理」も出る", async () => {
    state.authUser = { id: "1", canModerate: true };
    const page = await show();
    expect(page.textContent).toContain("Moderate shops");
  });
});

describe("ShopListPage の空の一覧", () => {
  it("0 件のときは、遊びの文言の画面を出す", async () => {
    state.shops = [];
    const page = await show();
    expect(page.textContent).toContain("Nobody's eaten here yet");
  });
});

describe("ShopListPage の初期の keyword", () => {
  // トップページの検索欄から /shops?keyword=... で移動したとき、絞り込みの入力欄に反映される(URL からの初期値)。
  it("URL の keyword クエリを、検索欄の初期値にする", async () => {
    const page = await show(["/shops?keyword=Shake"]);
    const input = page.querySelector<HTMLInputElement>("input[type='text']");
    expect(input?.value).toBe("Shake");
  });
});

it("記録入口で審査待ちの店舗を選ぶと投稿フォームではなく店舗詳細へ進む", async () => {
  state.authUser = { id: "1", canModerate: false };
  state.shops = [{ id: "7", name: "Shake Shack", status: "pending", photoUrl: null, averageRating: null, reviewCount: 0 }];
  const page = await mount(<MemoryRouter initialEntries={["/record"]}><RecordPage /></MemoryRouter>);
  expect(page.querySelector('a[href="/shops/7?from=record"]')).not.toBeNull();
  expect(page.querySelector('a[href^="/reviews/new"]')).toBeNull();
});

describe("ShopListPage の都道府県の絞り込み", () => {
  const prefectureSelect = (page: HTMLElement) =>
    need(page.querySelector<HTMLSelectElement>('select[aria-label="Filter by prefecture"]'), "都道府県の select");
  const lastParams = () => useShopsParams.mock.lastCall?.[0] as { keyword?: string; prefectureCode?: string };

  it("先頭は「すべての都道府県」で、続いて GET /meta の都道府県が並ぶ(日本語表示では日本語名)", async () => {
    await i18n.changeLanguage("ja");
    const page = await show();
    const select = need(page.querySelector<HTMLSelectElement>('select[aria-label="都道府県で絞り込む"]'), "都道府県の select");
    expect([...select.options].map((o) => o.textContent)).toEqual(["すべての都道府県", "北海道", "東京都"]);
    expect(select.value).toBe("");
  });

  it("東京都を選ぶと、一覧の取得に都道府県のコード 13 を渡し、「すべての都道府県」に戻すと外す", async () => {
    const page = await show();
    expect(lastParams().prefectureCode).toBe("");

    await choose(prefectureSelect(page), "13");
    expect(lastParams().prefectureCode).toBe("13");

    await choose(prefectureSelect(page), "");
    expect(lastParams().prefectureCode).toBe("");
  });

  it("keyword と一緒に使える(両方を一覧の取得に渡す)", async () => {
    const page = await show();
    await type(need(page.querySelector<HTMLInputElement>("input[type='text']"), "検索欄"), "Shake");
    await choose(prefectureSelect(page), "13");
    expect(lastParams()).toMatchObject({ keyword: "Shake", prefectureCode: "13" });
  });
});
