// @vitest-environment jsdom
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import SignoutPage from "./SignoutPage";

const logout = vi.hoisted(() => vi.fn<() => Promise<void>>());
vi.mock("../AuthProvider", () => ({
  useAuth: () => ({ user: null, isLoading: false, logout }),
}));

beforeEach(() => {
  logout.mockReset();
  logout.mockResolvedValue(undefined);
});
afterEach(cleanup);

describe("SignoutPage(サインアウトの実行)", () => {
  it("StrictMode の二重マウントでも logout を 1 回だけ呼び、完了後にレビュー一覧へ移る", async () => {
    const page = await mount(
      <StrictMode>
        <MemoryRouter initialEntries={["/signout"]}>
          <Routes>
            <Route path="/signout" element={<SignoutPage />} />
            <Route path="/reviews" element={<p>reviews list</p>} />
          </Routes>
        </MemoryRouter>
      </StrictMode>,
    );

    expect(logout).toHaveBeenCalledTimes(1);
    expect(page.textContent).toBe("reviews list");
  });
});
