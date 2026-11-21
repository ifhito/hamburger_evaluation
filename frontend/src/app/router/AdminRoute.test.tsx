// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { cleanup, mount } from "../../test/dom";
import { AdminRoute } from "./AdminRoute";
import { returnPathFrom } from "./returnTo";

const auth = vi.hoisted(() => ({ value: { user: null, isLoading: false } as { user: { canModerate: boolean } | null; isLoading: boolean } }));
vi.mock("../../domains/auth/AuthProvider", () => ({ useAuth: () => auth.value }));

function SigninCapture() {
  const location = useLocation();
  return <p>sign in:{returnPathFrom(location.state) ?? "none"}</p>;
}

const show = (initial = "/admin/shops?status=pending") =>
  mount(
    <MemoryRouter initialEntries={[initial]}>
      <Routes>
        <Route element={<AdminRoute />}>
          <Route path="/admin/shops" element={<p>admin screen</p>} />
        </Route>
        <Route path="/shops" element={<p>shop list</p>} />
        <Route path="/signin" element={<SigninCapture />} />
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

  it("サインインしていない人は、元の管理画面のパスとクエリを戻り先(state.from)に添えてサインイン画面へ送る", async () => {
    expect((await show()).textContent).toBe("sign in:/admin/shops?status=pending");
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
