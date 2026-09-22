// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { StrictMode, act } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { Provider, createStore } from "jotai";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, eventually, mount, need } from "../../../test/dom";
import { AuthProvider } from "../AuthProvider";
import { authApi } from "../api/authApiClient";
import SignupConfirmPage from "./SignupConfirmPage";

vi.mock("../api/authApiClient", () => ({ authApi: { confirmSignup: vi.fn(), me: vi.fn() } }));
const confirmSignup = vi.mocked(authApi.confirmSignup);

function Probe() {
  return <p data-testid="probe">{useLocation().pathname}</p>;
}

const show = () =>
  mount(
    <StrictMode>
      <Provider store={createStore()}>
        <AuthProvider>
          <MemoryRouter initialEntries={["/signup/confirm?token=mail-token"]}>
            <Routes>
              <Route path="/signup/confirm" element={<SignupConfirmPage />} />
              <Route path="*" element={<Probe />} />
            </Routes>
          </MemoryRouter>
        </AuthProvider>
      </Provider>
    </StrictMode>,
  );

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
});
afterEach(cleanup);

describe("SignupConfirmPage(確認メールのリンクの受け皿)", () => {
  it("確認している間は、見出しと「少しお待ちください」を出し、読み上げの対象(role=status)にも、同じ内容を伝える", async () => {
    confirmSignup.mockReturnValue(new Promise(() => {}));
    const page = await show();

    expect(page.querySelector("h1")?.textContent).toBe("Confirming your email address…");
    expect(page.textContent).toContain("Just a moment");
    const status = need(page.querySelector("[role=status]"), "status");
    // 空のまま先に描いてから、次の描画で埋める(中身の変化として気づかせるため)ので、埋まるまで待つ。
    await eventually(() => expect(status.textContent).toContain("Confirming your email address…"));
    expect(status.textContent).toContain("Just a moment");
  });

  it("リンクのトークンで、確認を 1 回だけ呼び(StrictMode の二重実行でも)、成功したら一覧へ移動する", async () => {
    const pending = deferred<Awaited<ReturnType<typeof authApi.confirmSignup>>>();
    confirmSignup.mockReturnValue(pending.promise);
    const page = await show();

    await act(async () => pending.resolve({ id: "7", username: "carol", email: "carol@example.com", canModerate: false, token: "jwt" }));

    expect(confirmSignup).toHaveBeenCalledTimes(1);
    expect(confirmSignup).toHaveBeenCalledWith("mail-token");
    await eventually(() => expect(page.querySelector("[data-testid=probe]")?.textContent).toBe("/reviews"));
  });

  it("確認できなかったとき(期限切れ・無効)は、API が返した文言を、そのまま、赤いエラーで出し、もう一度の新規登録へ案内する", async () => {
    confirmSignup.mockRejectedValue(new ApiError(["Confirmation token is invalid or has expired"], 400));
    const page = await show();

    await eventually(() => expect(page.querySelector("[role=alert]")).not.toBeNull());
    expect(page.querySelector("h1")?.textContent).toBe("Could not confirm your email address");
    const alert = need(page.querySelector("[role=alert]"), "alert");
    expect(alert.textContent).toContain("Confirmation failed");
    expect(alert.textContent).toContain("Confirmation token is invalid or has expired");
    expect(byText(page, "a", "Sign up again")?.getAttribute("href")).toBe("/signup");
    expect(page.querySelector("[role=status]")).toBeNull();
  });
});
