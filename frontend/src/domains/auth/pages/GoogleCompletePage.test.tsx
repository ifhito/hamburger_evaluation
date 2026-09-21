// @vitest-environment jsdom
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { BrowserRouter, MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { Provider, createStore } from "jotai";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need, type as typeInto } from "../../../test/dom";
import { AuthProvider, useAuth } from "../AuthProvider";
import { authApi } from "../api/authApiClient";
import type { GoogleExchangeResponse } from "../types";
import GoogleCompletePage from "./GoogleCompletePage";
import SigninPage from "./SigninPage";

vi.mock("../api/authApiClient", () => ({
  authApi: { exchangeGoogleCode: vi.fn(), login: vi.fn(), me: vi.fn() },
}));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { loginProviders: ["google"] } }) }));

const exchangeGoogleCode = vi.mocked(authApi.exchangeGoogleCode);
const login = vi.mocked(authApi.login);
const me = vi.mocked(authApi.me);

const signedIn = (returnTo: string): GoogleExchangeResponse => ({
  id: "7",
  username: "carol",
  email: "carol@gmail.example",
  canModerate: false,
  token: "jwt-from-google",
  returnTo,
});
const passwordLogin = { id: "7", username: "carol", email: "carol@gmail.example", canModerate: false, token: "jwt-from-password" };
const CONFLICT = "An account with this email address already exists. Sign in with your password, then connect Google from your profile.";

// いまの場所と、ログイン中の利用者を、文字として出す(遷移と認証の状態を確かめるため)。
function Probe() {
  const { pathname, search } = useLocation();
  const { user } = useAuth();
  return (
    <p data-testid="probe">
      {pathname}
      {search}|{user?.username ?? "-"}
    </p>
  );
}
const probeOf = (page: HTMLElement) => page.querySelector("[data-testid=probe]")?.textContent;

// 認証の状態(jotai)は、テストごとに新しくする。
function Shell({ children }: { children: React.ReactNode }) {
  return (
    <Provider store={createStore()}>
      <AuthProvider>{children}</AuthProvider>
    </Provider>
  );
}

const routes = (
  <Routes>
    <Route path="/auth/google/complete" element={<GoogleCompletePage />} />
    <Route path="/signin" element={<SigninPage />} />
    <Route path="*" element={<Probe />} />
  </Routes>
);

// ブラウザで開いたときの URL(Google から戻ってきた直後)。router は、この URL から始める。
function openCompleteUrl(search = "?code=one-time-code") {
  window.history.replaceState(null, "", `/auth/google/complete${search}`);
}

const showBrowser = () =>
  mount(
    <BrowserRouter>
      <Shell>{routes}</Shell>
    </BrowserRouter>,
  );

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  openCompleteUrl();
});
afterEach(cleanup);

describe("GoogleCompletePage(Google から戻ったあとの、コードの交換)", () => {
  it("コードを 1 回だけ交換して、サインインした状態にし、backend が返した戻り先へ移動する", async () => {
    exchangeGoogleCode.mockResolvedValue(signedIn("/shops/3"));
    const page = await showBrowser();

    await eventually(() => expect(probeOf(page)).toBe("/shops/3|carol"));
    expect(exchangeGoogleCode).toHaveBeenCalledTimes(1);
    expect(exchangeGoogleCode).toHaveBeenCalledWith("one-time-code");
    expect(localStorage.getItem("token")).toBe("jwt-from-google");
    expect(window.location.pathname).toBe("/shops/3");
  });

  it("戻り先が空のときは、既定の画面(/reviews)へ移動する", async () => {
    exchangeGoogleCode.mockResolvedValue(signedIn(""));
    const page = await showBrowser();

    await eventually(() => expect(probeOf(page)).toBe("/reviews|carol"));
  });

  it("戻り先がアプリの外を指していたら、既定の画面へ移動する(外へは移動しない)", async () => {
    exchangeGoogleCode.mockResolvedValue(signedIn("//evil.example/steal"));
    const page = await showBrowser();

    await eventually(() => expect(probeOf(page)).toBe("/reviews|carol"));
  });

  it("結び付けが成功したときは、サインインの状態は変えず(トークンを保存せず)、戻り先へ移動する", async () => {
    exchangeGoogleCode.mockResolvedValue({ linked: true, returnTo: "/users/7" });
    const page = await showBrowser();

    await eventually(() => expect(probeOf(page)).toBe("/users/7|-"));
    expect(localStorage.getItem("token")).toBeNull();
  });

  it("StrictMode(開発時の二重実行)でも、コードの交換は 1 回だけ", async () => {
    exchangeGoogleCode.mockResolvedValue(signedIn("/reviews"));
    const page = await mount(
      <StrictMode>
        <BrowserRouter>
          <Shell>{routes}</Shell>
        </BrowserRouter>
      </StrictMode>,
    );

    await eventually(() => expect(probeOf(page)).toBe("/reviews|carol"));
    expect(exchangeGoogleCode).toHaveBeenCalledTimes(1);
  });

  it("コードは、交換の結果を待たずに URL から消し、履歴を増やさない", async () => {
    exchangeGoogleCode.mockReturnValue(new Promise(() => undefined));
    const before = window.history.length;
    await showBrowser();

    expect(window.location.search).toBe("");
    expect(window.location.href).not.toContain("one-time-code");
    expect(window.history.length).toBe(before);
    expect(exchangeGoogleCode).toHaveBeenCalledWith("one-time-code");
  });

  it("交換に失敗したとき(サーバーの文言)は、その文言と、サインインへ戻るリンクを出し、サインインはしない", async () => {
    exchangeGoogleCode.mockRejectedValue(new ApiError([CONFLICT], 409));
    const page = await showBrowser();

    await eventually(() => expect(page.textContent).toContain(CONFLICT));
    expect(byText(page, "a", "Back to sign in")?.getAttribute("href")).toBe("/signin");
    expect(localStorage.getItem("token")).toBeNull();
  });

  it("サーバーの文言がない失敗(通信の失敗など)は、予備の文言を出す", async () => {
    exchangeGoogleCode.mockRejectedValue(new Error("network down"));
    const page = await showBrowser();

    await eventually(() => expect(page.textContent).toContain("Google sign-in failed. Please try again."));
  });

  it("ログイン中の利用者の手続きが失敗したときは、サインインではなく、プロフィールへ戻るリンクを出す", async () => {
    localStorage.setItem("token", "jwt-existing");
    me.mockResolvedValue({ id: "7", username: "carol", email: "carol@gmail.example", canModerate: false });
    exchangeGoogleCode.mockRejectedValue(new ApiError(["This Google account is already connected to another user."], 409));
    const page = await showBrowser();

    await eventually(() => expect(byText(page, "a", "Back to your profile")?.getAttribute("href")).toBe("/users/7"));
    expect(byText(page, "a", "Back to sign in")).toBeUndefined();
  });
});

describe("Google の手続きが失敗したあとの、元の画面への戻り(許可の画面から始めたとき)", () => {
  const CONSENT = "/oauth/authorize?client_id=app-1&state=xyz";

  // backend は、失敗の応答(409・400)に、検証済みの戻り先(return_to)を含める。画面は、それをサインインの画面へ渡すだけ。
  async function failWith(returnTo: string | undefined) {
    exchangeGoogleCode.mockRejectedValue(new ApiError([CONFLICT], 409, returnTo));
    return mount(
      <MemoryRouter initialEntries={["/auth/google/complete?code=one-time-code"]}>
        <Shell>{routes}</Shell>
      </MemoryRouter>,
    );
  }

  async function backToSigninAndLogIn(page: HTMLElement) {
    await eventually(() => expect(page.textContent).toContain(CONFLICT));
    await click(need(byText(page, "a", "Back to sign in"), "Back to sign in"));
    await eventually(() => expect(byText(page, "button", "Sign in")).toBeDefined());
    await typeInto(need(page.querySelector<HTMLInputElement>("#email"), "#email"), "carol@gmail.example");
    await typeInto(need(page.querySelector<HTMLInputElement>("#password"), "#password"), "correct horse battery");
    await click(need(byText(page, "button", "Sign in"), "Sign in"));
  }

  it("失敗の応答が戻り先(許可の画面)を返したとき、案内どおりパスワードでサインインすると、その許可の画面へ戻る", async () => {
    login.mockResolvedValue(passwordLogin);
    const page = await failWith(CONSENT);

    await backToSigninAndLogIn(page);

    await eventually(() => expect(probeOf(page)).toBe(`${CONSENT}|carol`));
  });

  it("失敗の応答に戻り先がない(空・なし)ときは、サインインのあと、既定の画面(/reviews)へ行く", async () => {
    for (const returnTo of [undefined, ""]) {
      login.mockResolvedValue(passwordLogin);
      const page = await failWith(returnTo);

      await backToSigninAndLogIn(page);

      await eventually(() => expect(probeOf(page)).toBe("/reviews|carol"));
      await cleanup();
      localStorage.clear(); // 次の繰り返しは、サインイン前の状態から始める
    }
  });

  it("失敗の応答の戻り先が、アプリの外を指していたら、サインイン画面へ渡さない(既定の画面へ行く)", async () => {
    login.mockResolvedValue(passwordLogin);
    const page = await failWith("https://evil.example/steal");

    await backToSigninAndLogIn(page);

    await eventually(() => expect(probeOf(page)).toBe("/reviews|carol"));
  });

  it("ログイン中の利用者の手続き(結び付け)が失敗したときは、戻り先があっても、プロフィールへ戻るリンクを出す", async () => {
    localStorage.setItem("token", "jwt-existing");
    me.mockResolvedValue({ id: "7", username: "carol", email: "carol@gmail.example", canModerate: false });
    const page = await failWith("/users/7");

    await eventually(() => expect(byText(page, "a", "Back to your profile")?.getAttribute("href")).toBe("/users/7"));
    expect(byText(page, "a", "Back to sign in")).toBeUndefined();
  });
});
