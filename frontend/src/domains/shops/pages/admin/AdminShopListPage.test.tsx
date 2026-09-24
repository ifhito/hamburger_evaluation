// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { MemoryRouter } from "react-router-dom";
import "../../../../lib/i18n";
import { ApiError } from "../../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need } from "../../../../test/dom";
import type { AdminShop } from "../../api/types";
import AdminShopListPage from "./AdminShopListPage";

const state = vi.hoisted(() => ({ shops: [] as AdminShop[], isLoading: false, statusAsked: [] as (string | undefined)[] }));
const approve = vi.hoisted(() => vi.fn());
const reject = vi.hoisted(() => vi.fn());
vi.mock("../../hooks/useShopMutations", () => ({
  useAdminShops: (status?: string) => {
    state.statusAsked.push(status);
    return { data: state.isLoading ? undefined : state.shops, isLoading: state.isLoading };
  },
  useShopModeration: () => ({ approve, reject }),
}));
vi.mock("../../../../api/meta", () => ({ useMeta: () => ({ data: { text: { moderationNoteMaxChars: 500 } } }) }));
vi.mock("../../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "1", username: "admin", canModerate: true }, isLoading: false }) }));

const shop = (over: Partial<AdminShop>): AdminShop => ({
  id: "s1",
  name: "Shop One",
  status: "pending",
  photoUrl: null,
  averageRating: null,
  reviewCount: 0,
  moderationNote: null,
  creator: { id: "u1", username: "alice" },
  canApprove: true,
  canReject: true,
  ...over,
});
const show = () =>
  mount(
    <MemoryRouter>
      <AdminShopListPage />
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.resetAllMocks();
  state.shops = [];
  state.isLoading = false;
  state.statusAsked = [];
});
afterEach(cleanup);

describe("AdminShopListPage(ショップの管理の一覧)", () => {
  it("承認・却下のボタンは、ショップごとに backend が返す canApprove・canReject だけで出し分ける(状態からは決めない)", async () => {
    state.shops = [
      shop({ id: "a", name: "Pending", status: "pending", canApprove: true, canReject: true }),
      // 公開中なのに、canApprove が true・canReject が false の、作り物の組み合わせ(状態で決めていれば、逆になる)
      shop({ id: "b", name: "Odd", status: "active", canApprove: true, canReject: false }),
      shop({ id: "c", name: "Done", status: "rejected", canApprove: false, canReject: false }),
    ];
    const page = await show();

    const rows = [...page.querySelectorAll("article")];
    const labels = (row: Element) => [...row.querySelectorAll("button, a")].map((el) => el.textContent);
    expect(labels(rows[0])).toEqual(["Edit", "Approve", "Reject"]);
    expect(labels(rows[1])).toEqual(["Edit", "Approve"]);
    expect(labels(rows[2])).toEqual(["Edit"]);
  });

  it("状態は文字の札で出し(公開中だけ黄色の札)、作った人と、却下の理由(あれば)を出す", async () => {
    state.shops = [shop({ status: "rejected", moderationNote: "Name mismatch", canReject: false })];
    const page = await show();

    expect(page.textContent).toContain("Rejected");
    expect(page.textContent).toContain("Created by: alice");
    expect(page.textContent).toContain("Reason: Name mismatch");
  });

  it("「Edit」は、編集の画面へのリンク(ボタンをリンクの中に入れない)", async () => {
    state.shops = [shop({ id: "s9" })];
    const page = await show();

    const link = need(page.querySelector<HTMLAnchorElement>('a[href="/admin/shops/s9/edit"]'), "edit link");
    expect(link.textContent).toBe("Edit");
    expect(link.querySelector("button")).toBeNull();
  });

  it("絞り込み: 選んでいるものが押された状態(aria-pressed)になり、その状態で一覧を取り直す。件数も出す", async () => {
    state.shops = [shop({ id: "a" }), shop({ id: "b" })];
    const page = await show();
    expect(page.textContent).toContain("2 shops");
    expect(need(byText(page, "button", "All"), "All").getAttribute("aria-pressed")).toBe("true");

    await click(need(byText(page, "button", "Pending"), "Pending"));

    expect(need(byText(page, "button", "Pending"), "Pending").getAttribute("aria-pressed")).toBe("true");
    expect(need(byText(page, "button", "All"), "All").getAttribute("aria-pressed")).toBe("false");
    expect(state.statusAsked[state.statusAsked.length - 1]).toBe("pending");
  });

  it("絞り込みを変えると、開いていた却下の理由の欄と、却下の失敗の表示を閉じる(対象のショップが一覧から消えることがあるため)", async () => {
    state.shops = [shop({ id: "a" })];
    reject.mockRejectedValue(new ApiError(["Moderation note is too long"], 422));
    const page = await show();
    await click(need(byText(page, "button", "Reject"), "Reject"));
    await click(need(byText(page, "button", "Confirm reject"), "Confirm reject"));
    await eventually(() => expect(page.querySelector('[role="alert"]')).not.toBeNull());

    await click(need(byText(page, "button", "Pending"), "Pending"));

    expect(page.querySelector("textarea")).toBeNull();
    expect(page.querySelector('[role="alert"]')).toBeNull();
  });

  it("「Reject」を押すと理由の欄が出て(その行の操作は隠れる)、「Confirm reject」で、理由つきで却下を送る", async () => {
    state.shops = [shop({ id: "a" })];
    reject.mockResolvedValue(undefined);
    const page = await show();

    await click(need(byText(page, "button", "Reject"), "Reject"));
    expect(byText(page, "button", "Approve")).toBeUndefined();
    await eventually(() => expect(page.querySelector("textarea")).not.toBeNull());
    const area = need(page.querySelector("textarea"), "textarea");
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(area, "Not a real shop");
      area.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await click(need(byText(page, "button", "Confirm reject"), "Confirm reject"));

    expect(reject).toHaveBeenCalledWith("a", "Not a real shop");
    await eventually(() => expect(page.querySelector("textarea")).toBeNull());
  });

  it("却下に失敗したとき(API の文言)は、画面の見出しと、その文言を出し、理由の欄は開いたままにする", async () => {
    state.shops = [shop({ id: "a" })];
    reject.mockRejectedValue(new ApiError(["Moderation note is too long"], 422));
    const page = await show();
    await click(need(byText(page, "button", "Reject"), "Reject"));

    await click(need(byText(page, "button", "Confirm reject"), "Confirm reject"));

    await eventually(() => expect(page.querySelector('[role="alert"]')?.textContent).toContain("Moderation note is too long"));
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Could not reject the shop");
    expect(page.querySelector("textarea")).not.toBeNull();
  });

  it("「Cancel」で理由の欄を閉じ、送らない", async () => {
    state.shops = [shop({ id: "a" })];
    const page = await show();
    await click(need(byText(page, "button", "Reject"), "Reject"));

    await click(need(byText(page, "button", "Cancel"), "Cancel"));

    expect(page.querySelector("textarea")).toBeNull();
    expect(reject).not.toHaveBeenCalled();
    expect(byText(page, "button", "Reject")).toBeDefined();
  });

  it("「Approve」を押すと、そのショップの承認を送る", async () => {
    state.shops = [shop({ id: "a" })];
    approve.mockResolvedValue(undefined);
    const page = await show();

    await click(need(byText(page, "button", "Approve"), "Approve"));

    expect(approve).toHaveBeenCalledWith("a");
  });

  it("承認に失敗したとき(API の文言)は、画面の見出しと、その文言を出す", async () => {
    state.shops = [shop({ id: "a" })];
    approve.mockRejectedValue(new ApiError(["Shop is already approved"], 422));
    const page = await show();

    await click(need(byText(page, "button", "Approve"), "Approve"));

    await eventually(() => expect(page.querySelector('[role="alert"]')?.textContent).toContain("Shop is already approved"));
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Could not approve the shop");
  });

  it("行ごとの処理中は独立している: A の却下が進行中でも、B の承認を始めたことで A の読み込み中の表示は消えない", async () => {
    state.shops = [shop({ id: "a", name: "Shop A" }), shop({ id: "b", name: "Shop B" })];
    let resolveReject: (() => void) | undefined;
    reject.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveReject = resolve;
        }),
    );
    approve.mockResolvedValue(undefined);
    const page = await show();
    const rows = [...page.querySelectorAll("article")];

    await click(need(byText(rows[0], "button", "Reject"), "Reject(A)"));
    const confirmButton = need(byText<HTMLButtonElement>(rows[0], "button", "Confirm reject"), "Confirm reject(A)");
    await click(confirmButton);
    expect(confirmButton.disabled).toBe(true);
    expect(confirmButton.getAttribute("aria-busy")).toBe("true");

    // B の承認を始めても、A の却下の読み込み中の表示が消えてはいけない(別の行の busy 状態を巻き込んで消さない)。
    await click(need(byText(rows[1], "button", "Approve"), "Approve(B)"));

    expect(confirmButton.disabled).toBe(true);
    expect(confirmButton.getAttribute("aria-busy")).toBe("true");
    expect(approve).toHaveBeenCalledWith("b");

    await act(async () => {
      resolveReject?.();
    });
    await eventually(() => expect(rows[0].querySelector("textarea")).toBeNull());
  });

  it("0 件のとき: 審査待ちの絞り込みには専用の空の画面、そのほかの絞り込みには「No shops.」を出す", async () => {
    const page = await show();
    expect(page.textContent).toContain("No shops.");
    expect(page.textContent).not.toContain("Nothing is waiting for review");

    await click(need(byText(page, "button", "Pending"), "Pending"));

    expect(page.textContent).toContain("Nothing is waiting for review");
    expect(page.textContent).not.toContain("No shops.");
  });

  it("取得中は、読み込みの表示を出す", async () => {
    state.isLoading = true;
    const page = await show();

    expect(page.querySelector('[role="status"]')).not.toBeNull();
  });
});
