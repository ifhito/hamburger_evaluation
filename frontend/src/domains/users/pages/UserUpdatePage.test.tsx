// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount, need, type } from "../../../test/dom";
import type { User } from "../api/types";
import UserUpdatePage from "./UserUpdatePage";

// フォームの初期値・変更検知は、取得済みの profile(useUser)から取り、AuthProvider の authUser(古いキャッシュのことがある)
// からは取らないことを確かめる。あわせて、読み込み中・取得失敗・canEdit が false のときの出し分けも確かめる。
const state = vi.hoisted(() => ({ profile: undefined as User | undefined, error: undefined as unknown, isLoading: false }));
const update = vi.hoisted(() => vi.fn());
vi.mock("../../auth/AuthProvider", () => ({
  useAuth: () => ({
    user: { id: "1", username: "stale-name", email: "stale@example.com" },
    isLoading: false,
    logout: vi.fn(),
    refreshUser: vi.fn(),
  }),
}));
vi.mock("../hooks/useUser", () => ({ useUser: () => ({ data: state.profile, error: state.error, isLoading: state.isLoading }) }));
vi.mock("../hooks/useUserMutations", () => ({
  useUpdateUser: () => ({ update }),
  useDeleteUser: () => ({ destroy: vi.fn() }),
}));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { text: { usernameMaxChars: 50, bioMaxChars: 500 }, password: { minBytes: 8, maxBytes: 72 } } }) }));

const baseProfile: User = { id: "1", username: "fresh-name", bio: "fresh bio", email: "fresh@example.com", canEdit: true };

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/users/1/edit"]}>
      <Routes>
        <Route path="/users/:id/edit" element={<UserUpdatePage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  state.profile = baseProfile;
  state.error = undefined;
  state.isLoading = false;
  update.mockReset();
  update.mockResolvedValue({ username: "fresh-name", email: "fresh@example.com" });
});
afterEach(cleanup);

describe("UserUpdatePage の初期値", () => {
  it("username / email は、AuthProvider の authUser ではなく、取得済みの profile の値にする", async () => {
    const page = await show();
    const username = page.querySelector<HTMLInputElement>("#username");
    const email = page.querySelector<HTMLInputElement>("#email");
    expect(username?.value).toBe("fresh-name");
    expect(email?.value).toBe("fresh@example.com");
  });
});

describe("UserUpdatePage の読み込み中・取得に失敗したとき", () => {
  it("読み込み中はフォームを出さず、読み込みの表示を出す", async () => {
    state.isLoading = true;
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.querySelector('[role="status"]')).not.toBeNull();
  });

  it("取得に失敗したときはフォームを出さず、失敗の文言を出す", async () => {
    state.profile = undefined;
    state.error = new Error("failed");
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.textContent).toContain("Failed to load user.");
  });
});

describe("UserUpdatePage の送信", () => {
  it("username だけ変えて送ると、変えた項目だけを更新する(触っていない bio / email は送らない)", async () => {
    const page = await show();
    const username = need(page.querySelector<HTMLInputElement>("#username"), "username");
    await type(username, "new-name");

    await act(async () => {
      need(page.querySelector("form"), "form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });

    expect(update).toHaveBeenCalledWith({ username: "new-name" });
  });
});

describe("UserUpdatePage の canEdit", () => {
  it("canEdit が false のときは、フォームを出さず、編集できない旨を出す", async () => {
    state.profile = { ...baseProfile, canEdit: false };
    const page = await show();
    expect(page.querySelector("form")).toBeNull();
    expect(page.textContent).toContain("You can't edit this profile");
  });
});
