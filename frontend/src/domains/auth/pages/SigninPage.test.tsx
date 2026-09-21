// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { Provider, createStore } from "jotai";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need, type as typeInto } from "../../../test/dom";
import { AuthProvider } from "../AuthProvider";
import { authApi } from "../api/authApiClient";
import SigninPage from "./SigninPage";

vi.mock("../api/authApiClient", () => ({ authApi: { login: vi.fn(), me: vi.fn() } }));
// GET /meta の内容は、テストごとに差し替える。
const meta = vi.hoisted(() => ({ data: { loginProviders: ["google"] } as { loginProviders: string[] } }));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: meta.data }) }));

const login = vi.mocked(authApi.login);

const show = () =>
  mount(
    <Provider store={createStore()}>
      <AuthProvider>
        <MemoryRouter initialEntries={["/signin"]}>
          <SigninPage />
        </MemoryRouter>
      </AuthProvider>
    </Provider>,
  );

async function submit(page: HTMLElement) {
  await typeInto(need(page.querySelector<HTMLInputElement>("#email"), "#email"), "alice@example.com");
  await typeInto(need(page.querySelector<HTMLInputElement>("#password"), "#password"), "password1");
  await click(need(byText(page, "button", "Sign in"), "Sign in"));
}

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  meta.data = { loginProviders: ["google"] };
});
afterEach(cleanup);

describe("SigninPage(サインインの画面)", () => {
  it("サインインに失敗したとき、API が返した文言を、そのまま、画面側の見出しつきの赤いエラーで出す", async () => {
    login.mockRejectedValue(new ApiError(["Too many attempts. Try again later"], 429));
    const page = await show();

    await submit(page);

    await eventually(() => expect(page.querySelector("[role=alert]")).not.toBeNull());
    const alert = need(page.querySelector("[role=alert]"), "alert");
    expect(alert.textContent).toContain("Could not sign in");
    expect(alert.textContent).toContain("Too many attempts. Try again later");
    expect(alert.textContent).not.toContain("Invalid email or password");
  });

  it("API の文言がない失敗(通信の失敗など)のときは、こちらの文言を出す", async () => {
    login.mockRejectedValue(new Error("network down"));
    const page = await show();

    await submit(page);

    await eventually(() => expect(page.querySelector("[role=alert]")?.textContent).toContain("Invalid email or password"));
  });

  it("GET /meta が Google を返しているときは、フォームの下に、「または」の区切りつきで、Google のボタンを出す", async () => {
    const page = await show();

    const google = need(byText(page, "a", "Continue with Google"), "Continue with Google");
    const submitButton = need(byText(page, "button", "Sign in"), "Sign in");
    expect(submitButton.compareDocumentPosition(google) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(byText(page, "p", "or")).toBeDefined();
  });

  it("GET /meta に Google がないときは、Google のボタンも「または」の区切りも出さない", async () => {
    meta.data = { loginProviders: [] };
    const page = await show();

    expect(byText(page, "a", "Continue with Google")).toBeUndefined();
    expect(byText(page, "p", "or")).toBeUndefined();
  });
});
