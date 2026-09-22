// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { cleanup, mount } from "../../test/dom";
import { AdminRoute } from "./AdminRoute";

const auth = vi.hoisted(() => ({ value: { user: null, isLoading: false } as { user: { canModerate: boolean } | null; isLoading: boolean } }));
vi.mock("../../domains/auth/AuthProvider", () => ({ useAuth: () => auth.value }));

const show = () =>
  mount(
    <MemoryRouter initialEntries={["/admin/shops"]}>
      <Routes>
        <Route element={<AdminRoute />}>
          <Route path="/admin/shops" element={<p>admin screen</p>} />
        </Route>
        <Route path="/shops" element={<p>shop list</p>} />
        <Route path="/signin" element={<p>sign in</p>} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  auth.value = { user: null, isLoading: false };
});
afterEach(cleanup);

describe("AdminRoute(管理の画面に入れる人)", () => {
  it("backend が canModerate を返した人だけが、管理の画面を開ける", async () => {
    auth.value = { user: { canModerate: true }, isLoading: false };

    expect((await show()).textContent).toBe("admin screen");
  });

  it("サインインしていない人は、サインインの画面へ送る", async () => {
    expect((await show()).textContent).toBe("sign in");
  });

  it("サインインしていても canModerate でない人は、ショップの一覧へ戻す(専用の「権限がありません」の画面は出さない)", async () => {
    auth.value = { user: { canModerate: false }, isLoading: false };

    expect((await show()).textContent).toBe("shop list");
  });

  it("認証の状態を復元している間は、何も出さず、どこへも送らない", async () => {
    auth.value = { user: null, isLoading: true };

    expect((await show()).textContent).toBe("");
  });
});
