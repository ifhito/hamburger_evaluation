// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../../lib/i18n";
import { ApiError } from "../../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need, type } from "../../../../test/dom";
import AdminShopEditPage from "./AdminShopEditPage";

// SWR は、バックグラウンドの再取得のたびに、値が同じでも新しい配列・新しいオブジェクトを返す(参照は変わる)。
// この mock も、呼ばれるたびに新しい配列・オブジェクトを作って、その挙動を再現する(同じ参照を使い回さない)。
const state = vi.hoisted(() => ({ isLoading: false, shop: { id: "s1", name: "Old name", status: "active" } }));
const update = vi.hoisted(() => vi.fn());
vi.mock("../../hooks/useShopMutations", () => ({
  useAdminShops: () => ({ data: state.isLoading ? undefined : [{ ...state.shop }], isLoading: state.isLoading }),
  useUpdateShop: () => ({ update }),
}));
vi.mock("../../../../api/meta", () => ({ useMeta: () => ({ data: { text: { shopNameMaxChars: 100 } } }) }));
vi.mock("../../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1", username: "admin", canModerate: true }, isLoading: false }) }));

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/admin/shops/s1/edit"]}>
      <Routes>
        <Route path="/admin/shops/:id/edit" element={<AdminShopEditPage />} />
        <Route path="/admin/shops" element={<p>moderation list</p>} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.resetAllMocks();
  state.isLoading = false;
});
afterEach(cleanup);

describe("AdminShopEditPage(ショップの名前の編集)", () => {
  it("いまの名前を入れた欄を出し、変えて保存すると、更新して、管理の一覧へ戻る", async () => {
    update.mockResolvedValue(undefined);
    const page = await show();
    const input = need(page.querySelector<HTMLInputElement>("#name"), "name");
    await eventually(() => expect(input.value).toBe("Old name"));

    await type(input, "New name");
    await act(async () => {
      need(page.querySelector("form"), "form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });

    await eventually(() => expect(update).toHaveBeenCalledWith({ name: "New name" }));
    await eventually(() => expect(page.textContent).toBe("moderation list"));
  });

  it("更新に失敗したとき(API の文言)は、画面の見出しと、その文言を出し、画面に留まる", async () => {
    update.mockRejectedValue(new ApiError(["Name is too long (maximum is 100 characters)"], 422));
    const page = await show();
    await eventually(() => expect(page.querySelector<HTMLInputElement>("#name")?.value).toBe("Old name"));

    await click(need(byText(page, "button", "Save"), "Save"));

    await eventually(() => expect(page.querySelector('[role="alert"]')?.textContent).toContain("Name is too long (maximum is 100 characters)"));
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Could not save your changes");
    expect(page.querySelector("form")).not.toBeNull();
  });

  it("同じショップを SWR が裏で取り直して、shop オブジェクトの参照だけが変わっても、入力中の値を無言で消さない", async () => {
    const page = await show();
    const input = need(page.querySelector<HTMLInputElement>("#name"), "name");
    await eventually(() => expect(input.value).toBe("Old name"));

    await type(input, "New name in progress");

    // 打った直後(useAdminShops はレンダーのたびに新しい配列・オブジェクトを返す)でも、消えていない
    expect(input.value).toBe("New name in progress");
  });

  it("戻るリンクと、キャンセルは、どちらも管理の一覧へ移るリンク", async () => {
    const page = await show();

    const links = [...page.querySelectorAll<HTMLAnchorElement>('a[href="/admin/shops"]')].map((a) => a.textContent);
    expect(links).toEqual(["← Back to shop moderation", "Cancel"]);
  });

  it("取得中は、読み込みの表示を出し、フォームは出さない", async () => {
    state.isLoading = true;
    const page = await show();

    expect(page.querySelector('[role="status"]')).not.toBeNull();
    expect(page.querySelector("form")).toBeNull();
  });
});
