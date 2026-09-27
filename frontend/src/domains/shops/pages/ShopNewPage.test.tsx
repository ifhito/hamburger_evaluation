// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, choose, cleanup, click, eventually, mount, need, type } from "../../../test/dom";
import ShopNewPage from "./ShopNewPage";

// ショップの申請で、住所の 3 項目が送信の本文に入ることと、サーバーの 422 をそのまま出すことを確かめる。
const create = vi.hoisted(() => vi.fn());
vi.mock("../hooks/useShopMutations", () => ({ useCreateShop: () => ({ create }) }));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1", username: "alice", canModerate: false }, isLoading: false }) }));
vi.mock("../../../api/meta", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../api/meta")>()),
  useMeta: () => ({
    data: {
      text: { shopNameMaxChars: 100, cityMaxChars: 100, streetAddressMaxChars: 200 },
      prefectures: [
        { code: 1, nameJa: "北海道", nameEn: "Hokkaido" },
        { code: 13, nameJa: "東京都", nameEn: "Tokyo" },
      ],
    },
  }),
}));

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/shops/new"]}>
      <Routes>
        <Route path="/shops/new" element={<ShopNewPage />} />
        <Route path="/shops/:id" element={<p>shop detail</p>} />
      </Routes>
    </MemoryRouter>,
  );

const input = (page: HTMLElement, id: string) => need(page.querySelector<HTMLInputElement>(`#${id}`), id);

beforeEach(() => {
  vi.resetAllMocks();
});
afterEach(cleanup);

describe("ShopNewPage の住所", () => {
  it("都道府県の選択欄は、先頭が未選択で、GET /meta の都道府県が続く", async () => {
    const page = await show();
    const select = need(page.querySelector<HTMLSelectElement>("#prefectureCode"), "都道府県");
    expect([...select.options].map((o) => o.textContent)).toEqual(["Not selected", "Hokkaido", "Tokyo"]);
    expect(select.value).toBe("");
  });

  it("都道府県・市区町村・番地以降を入れて申請すると、都道府県は数値のコードで送る", async () => {
    create.mockResolvedValue({ id: "s9" });
    const page = await show();
    await type(input(page, "name"), "Burger Joint");
    await choose(need(page.querySelector<HTMLSelectElement>("#prefectureCode"), "都道府県"), "13");
    await type(input(page, "city"), "渋谷区");
    await type(input(page, "streetAddress"), "神南1-2-3");
    await click(need(byText(page, "button", "Submit for review"), "申請"));

    await eventually(() =>
      expect(create).toHaveBeenCalledWith({
        name: "Burger Joint",
        mapUrl: "",
        prefectureCode: 13,
        city: "渋谷区",
        streetAddress: "神南1-2-3",
      }),
    );
    await eventually(() => expect(page.textContent).toBe("shop detail"));
  });

  it("都道府県を選ばずに申請すると、都道府県のコードは null で送る", async () => {
    create.mockResolvedValue({ id: "s9" });
    const page = await show();
    await type(input(page, "name"), "Burger Joint");
    await click(need(byText(page, "button", "Submit for review"), "申請"));

    await eventually(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({ prefectureCode: null, city: "", streetAddress: "" })));
  });

  it("サーバーが 422 を返したら、その文言をそのまま出し、画面に留まる", async () => {
    create.mockRejectedValue(new ApiError(["Prefecture is invalid", "City is too long (maximum is 100 characters)"], 422));
    const page = await show();
    await click(need(byText(page, "button", "Submit for review"), "申請"));

    await eventually(() => expect(page.querySelector('[role="alert"]')?.textContent).toContain("Prefecture is invalid"));
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("City is too long (maximum is 100 characters)");
    expect(page.querySelector("form")).not.toBeNull();
  });
});
