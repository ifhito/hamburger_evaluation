// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, useSyncExternalStore } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { ProtectedRoute } from "../../../app/router/ProtectedRoute";
import { byText, cleanup, click, mount, need, type } from "../../../test/dom";
import type { User } from "../api/types";
import UserUpdatePage from "./UserUpdatePage";

// フォームの初期値・変更検知は、取得済みの profile(useUser)から取り、AuthProvider の authUser(古いキャッシュのことがある)
// からは取らないことを確かめる。あわせて、読み込み中・取得失敗・canEdit が false のときの出し分けも確かめる。
const authListeners = vi.hoisted(() => new Set<() => void>());
const state = vi.hoisted(() => ({
  authUser: { id: "1", username: "stale-name", email: "stale@example.com" } as { id: string; username: string; email: string } | null,
  profile: undefined as User | undefined,
  error: undefined as unknown,
  isLoading: false,
}));
const update = vi.hoisted(() => vi.fn());
const destroy = vi.hoisted(() => vi.fn());
const logout = vi.hoisted(() => vi.fn());
const clearSession = vi.hoisted(() =>
  vi.fn(() => {
    state.authUser = null;
    for (const listener of authListeners) listener();
  }),
);
vi.mock("../../auth/AuthProvider", () => ({
  useAuth: () => {
    const user = useSyncExternalStore(
      (listener) => {
        authListeners.add(listener);
        return () => authListeners.delete(listener);
      },
      () => state.authUser,
    );
    return {
      user,
      isLoading: false,
      logout,
      clearSession,
      refreshUser: vi.fn(),
    };
  },
}));
vi.mock("../hooks/useUser", () => ({ useUser: () => ({ data: state.profile, error: state.error, isLoading: state.isLoading }) }));
vi.mock("../hooks/useUserMutations", () => ({
  useUpdateUser: () => ({ update }),
  useDeleteUser: () => ({ destroy }),
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
  state.authUser = { id: "1", username: "stale-name", email: "stale@example.com" };
  state.profile = baseProfile;
  state.error = undefined;
  state.isLoading = false;
  update.mockReset();
  update.mockResolvedValue({ username: "fresh-name", email: "fresh@example.com" });
  destroy.mockReset();
  destroy.mockResolvedValue(undefined);
  logout.mockReset();
  clearSession.mockClear();
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

describe("UserUpdatePage のアカウント削除", () => {
  it("ProtectedRoute の配下でアカウントを削除しても、/signin に奪われずレビュー一覧へ移り、不要な logout API は呼ばない", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    try {
      const page = await mount(
        <MemoryRouter initialEntries={["/users/1/edit"]}>
          <Routes>
            <Route element={<ProtectedRoute />}>
              <Route path="/users/:id/edit" element={<UserUpdatePage />} />
            </Route>
            <Route path="/reviews" element={<p>reviews screen</p>} />
            <Route path="/signin" element={<p>signin screen</p>} />
          </Routes>
        </MemoryRouter>,
      );

      await click(need(byText(page, "button", "Delete Account"), "削除ボタン"));

      expect(destroy).toHaveBeenCalledWith("1");
      expect(clearSession).toHaveBeenCalledTimes(1);
      expect(logout).not.toHaveBeenCalled();
      expect(page.textContent).toBe("reviews screen");
    } finally {
      confirmSpy.mockRestore();
    }
  });
});
