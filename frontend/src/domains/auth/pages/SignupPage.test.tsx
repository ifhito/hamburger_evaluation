// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { Provider, createStore } from "jotai";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need, type as typeInto } from "../../../test/dom";
import { AuthProvider } from "../AuthProvider";
import { authApi } from "../api/authApiClient";
import SignupPage from "./SignupPage";

vi.mock("../api/authApiClient", () => ({ authApi: { signup: vi.fn(), me: vi.fn() } }));
vi.mock("../../../api/meta", () => ({
  useMeta: () => ({ data: { loginProviders: ["google"], text: { usernameMaxChars: 50 }, password: { minBytes: 8, maxBytes: 72 } } }),
}));

afterEach(cleanup);

describe("SignupPage の Google のボタン", () => {
  it("新規登録の画面には、サインインと同じ「Continue with Google」を、入力欄より上に出す", async () => {
    const page = await mount(
      <Provider store={createStore()}>
        <AuthProvider>
          <MemoryRouter initialEntries={["/signup"]}>
            <SignupPage />
          </MemoryRouter>
        </AuthProvider>
      </Provider>,
    );

    const google = need(byText(page, "a", "Continue with Google"), "Continue with Google");
    const username = need(page.querySelector("#username"), "#username");
    expect(google.compareDocumentPosition(username) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(byText(page, "a", "Sign up with Google")).toBeUndefined();
  });
});

describe("SignupPage のエラーと、確認メールを送った画面", () => {
  const signup = vi.mocked(authApi.signup);
  const show = () =>
    mount(
      <Provider store={createStore()}>
        <AuthProvider>
          <MemoryRouter initialEntries={["/signup"]}>
            <SignupPage />
          </MemoryRouter>
        </AuthProvider>
      </Provider>,
    );
  const submit = async (page: HTMLElement) => {
    await typeInto(need(page.querySelector<HTMLInputElement>("#username"), "#username"), "alice");
    await typeInto(need(page.querySelector<HTMLInputElement>("#email"), "#email"), "alice@example.com");
    await typeInto(need(page.querySelector<HTMLInputElement>("#password"), "#password"), "abc123");
    await typeInto(need(page.querySelector<HTMLInputElement>("#passwordConfirmation"), "#passwordConfirmation"), "abc124");
    await click(need(byText(page, "button", "Create account"), "Create account"));
  };

  it("登録に失敗したとき、API が返した文言を、そのまま、一覧で、画面側の見出しつきの赤いエラーで出す", async () => {
    signup.mockRejectedValue(new ApiError(["Password is too short (minimum is 8 characters)", "Password confirmation doesn't match Password"], 422));
    const page = await show();

    await submit(page);

    await eventually(() => expect(page.querySelector("[role=alert]")).not.toBeNull());
    const alert = need(page.querySelector("[role=alert]"), "alert");
    expect(alert.textContent).toContain("Could not create your account");
    expect([...alert.querySelectorAll("li")].map((li) => li.textContent)).toEqual([
      "Password is too short (minimum is 8 characters)",
      "Password confirmation doesn't match Password",
    ]);
  });

  it("確認メールを送ったあとは、送り先を太字で示し、もう一度の新規登録に戻れる", async () => {
    signup.mockResolvedValue({ message: "ok" });
    const page = await show();

    await submit(page);

    await eventually(() => expect(page.querySelector("h1")?.textContent).toBe("Check your email"));
    expect(page.querySelector("b")?.textContent).toBe("alice@example.com");
    expect(page.textContent).toContain("Open the link in it to finish creating your account.");
    await click(need(byText(page, "button", "Sign up again"), "Sign up again"));
    expect(page.querySelector("h1")?.textContent).toBe("Sign up");
  });
});
