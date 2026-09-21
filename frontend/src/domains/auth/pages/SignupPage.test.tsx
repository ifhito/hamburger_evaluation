// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { Provider, createStore } from "jotai";
import "../../../lib/i18n";
import { byText, cleanup, mount } from "../../../test/dom";
import { AuthProvider } from "../AuthProvider";
import SignupPage from "./SignupPage";

vi.mock("../api/authApiClient", () => ({ authApi: { signup: vi.fn(), me: vi.fn() } }));
vi.mock("../../../api/meta", () => ({
  useMeta: () => ({ data: { loginProviders: ["google"], text: { usernameMaxChars: 50 }, password: { minBytes: 8, maxBytes: 72 } } }),
}));

afterEach(cleanup);

describe("SignupPage の Google のボタン", () => {
  it("新規登録の画面には、「Sign up with Google」(サインインの文言ではない)を出す", async () => {
    const page = await mount(
      <Provider store={createStore()}>
        <AuthProvider>
          <MemoryRouter initialEntries={["/signup"]}>
            <SignupPage />
          </MemoryRouter>
        </AuthProvider>
      </Provider>,
    );

    expect(byText(page, "a", "Sign up with Google")).toBeDefined();
    expect(byText(page, "a", "Sign in with Google")).toBeUndefined();
  });
});
