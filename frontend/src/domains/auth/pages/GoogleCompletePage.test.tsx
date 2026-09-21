// @vitest-environment jsdom
import { StrictMode, act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { BrowserRouter, MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { Provider, createStore } from "jotai";
import "../../../lib/i18n";
import { API_BASE_URL, ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need, type as typeInto } from "../../../test/dom";
import { AuthProvider, useAuth } from "../AuthProvider";
import { GoogleExchangeError, authApi } from "../api/authApiClient";
import type { GoogleExchangeResponse } from "../types";
import GoogleCompletePage from "./GoogleCompletePage";
import SigninPage from "./SigninPage";

// 通信(authApi)だけを差し替え、失敗の型(GoogleExchangeError)は、本物を使う。
vi.mock("../api/authApiClient", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/authApiClient")>()),
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

// メモリ上の router で、指定した場所(と、遷移の state)から、画面を出す。
const showAt = (entry: string | { pathname: string; state: unknown }) =>
  mount(
    <MemoryRouter initialEntries={[entry]}>
      <Shell>{routes}</Shell>
    </MemoryRouter>,
  );

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
    exchangeGoogleCode.mockRejectedValue(new GoogleExchangeError(new ApiError([CONFLICT], 409, { returnTo })));
    return showAt("/auth/google/complete?code=one-time-code");
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

describe("サインイン画面の「Sign in with Google」(ログインが必要な画面から来たとき、戻り先を開始の URL に載せる)", () => {
  const CONSENT = "/oauth/authorize?client_id=app-1&state=xyz";
  const linkHref = async (entry: string | { pathname: string; state: unknown }) => {
    const page = await showAt(entry);
    return byText(page, "a", "Sign in with Google")?.getAttribute("href");
  };

  it("許可の画面などから送られてきたとき(state の from)は、その画面を return_to として載せる", async () => {
    expect(await linkHref({ pathname: "/signin", state: { from: CONSENT } })).toBe(`${API_BASE_URL}/auth/google/start?return_to=${encodeURIComponent(CONSENT)}`);
  });

  it("戻り先がないとき・アプリの外を指すときは、return_to を載せない", async () => {
    expect(await linkHref("/signin")).toBe(`${API_BASE_URL}/auth/google/start`);
    await cleanup();
    expect(await linkHref({ pathname: "/signin", state: { from: "https://evil.example/x" } })).toBe(`${API_BASE_URL}/auth/google/start`);
  });
});

// 交換の結果を、テストの好きな時点で決める(遅れて返す通信の代役)。
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("交換が一時的に失敗したとき(コードは、backend が消費していないので、同じコードで再試行できる)", () => {
  const showFromMemory = (entry = "/auth/google/complete?code=one-time-code") =>
    showAt(entry);

  it("サーバーの障害(500)・通信の失敗は、「Try again」を出し、押すと、同じコードでもう一度交換して、成功すればサインインする", async () => {
    for (const failure of [new ApiError(["internal server error"], 500), new Error("network down")]) {
      exchangeGoogleCode.mockReset();
      exchangeGoogleCode.mockRejectedValueOnce(failure).mockResolvedValueOnce(signedIn("/shops"));
      const page = await showFromMemory();
      await eventually(() => expect(byText(page, "button", "Try again")).toBeDefined());

      await click(need(byText(page, "button", "Try again"), "Try again"));

      await eventually(() => expect(probeOf(page)).toBe("/shops|carol"));
      expect(exchangeGoogleCode).toHaveBeenCalledTimes(2);
      expect(exchangeGoogleCode.mock.calls.map((c) => c[0])).toEqual(["one-time-code", "one-time-code"]);
      await cleanup();
      localStorage.clear();
    }
  });

  it("再試行も失敗したときは、また「Try again」が出る(何度でもやり直せる)", async () => {
    exchangeGoogleCode.mockRejectedValue(new ApiError(["internal server error"], 500));
    const page = await showFromMemory();
    await eventually(() => expect(byText(page, "button", "Try again")).toBeDefined());

    await click(need(byText(page, "button", "Try again"), "Try again"));

    await eventually(() => expect(exchangeGoogleCode).toHaveBeenCalledTimes(2));
    await eventually(() => expect(byText(page, "button", "Try again")).toBeDefined());
  });

  it("決まった失敗(409・400。コードは使い切られた)には、「Try again」を出さない", async () => {
    for (const status of [409, 400]) {
      exchangeGoogleCode.mockReset();
      exchangeGoogleCode.mockRejectedValue(new ApiError([CONFLICT], status));
      const page = await showFromMemory();
      await eventually(() => expect(page.textContent).toContain(CONFLICT));
      expect(byText(page, "button", "Try again"), String(status)).toBeUndefined();
      await cleanup();
    }
  });
});

describe("コードのない画面(URL から消したあとの、戻る操作など)", () => {
  it("コードがないときは、交換の要求を送らず、失敗の案内とサインインへの導線を出す", async () => {
    const page = await showAt("/auth/google/complete");

    await eventually(() => expect(page.textContent).toContain("Google sign-in failed. Please try again."));
    expect(exchangeGoogleCode).not.toHaveBeenCalled();
    expect(byText(page, "a", "Back to sign in")).toBeDefined();
    expect(byText(page, "button", "Try again")).toBeUndefined();
  });

  it("コードを URL から消すのは、router を通して行う(履歴の書き換えは、router の位置と食い違わない)", async () => {
    exchangeGoogleCode.mockReturnValue(new Promise(() => undefined));
    const page = await mount(
      <MemoryRouter initialEntries={["/auth/google/complete?code=one-time-code"]}>
        <Shell>
          <Routes>
            <Route path="/auth/google/complete" element={<><GoogleCompletePage /><Probe /></>} />
          </Routes>
        </Shell>
      </MemoryRouter>,
    );

    await eventually(() => expect(exchangeGoogleCode).toHaveBeenCalledWith("one-time-code"));
    expect(probeOf(page)).toBe("/auth/google/complete|-"); // router の位置から、query が消えている
  });
});

describe("画面を離れたあと・ログインの状態の復元中の、交換の結果", () => {
  it("「Signing you in…」の間に別の画面へ移ったら、あとで成功しても、勝手に移動させない(サインインは反映する)", async () => {
    const pending = deferred<GoogleExchangeResponse>();
    exchangeGoogleCode.mockReturnValue(pending.promise);
    const page = await showAt("/auth/google/complete?code=one-time-code");
    await eventually(() => expect(exchangeGoogleCode).toHaveBeenCalledTimes(1));
    await click(need(byText(page, "a", "Shops"), "Shops")); // ヘッダーのリンクで、別の画面へ

    await act(async () => pending.resolve(signedIn("/reviews")));

    await eventually(() => expect(localStorage.getItem("token")).toBe("jwt-from-google"));
    expect(probeOf(page)).toBe("/shops|carol"); // /reviews へ移動させられていない。サインインは反映されている
  });

  it("ログイン中の利用者の手続きの失敗は、ログインの状態を復元し終えるまで、リンクを出さず、復元できたら、プロフィールへ戻るリンクを出す(サインインへの誤った導線を出さない)", async () => {
    localStorage.setItem("token", "jwt-existing");
    const restoring = deferred<Awaited<ReturnType<typeof authApi.me>>>();
    me.mockReturnValue(restoring.promise);
    exchangeGoogleCode.mockRejectedValue(new ApiError(["This Google account is already connected to another user."], 409));
    const page = await showAt("/auth/google/complete?code=one-time-code");
    await eventually(() => expect(page.textContent).toContain("already connected to another user"));

    expect(byText(page, "a", "Back to sign in")).toBeUndefined(); // 復元の前は、どちらも出さない
    expect(byText(page, "a", "Back to your profile")).toBeUndefined();
    await act(async () => restoring.resolve({ id: "7", username: "carol", email: "carol@gmail.example", canModerate: false }));

    await eventually(() => expect(byText(page, "a", "Back to your profile")?.getAttribute("href")).toBe("/users/7"));
    expect(byText(page, "a", "Back to sign in")).toBeUndefined();
  });
});

describe("交換に成功したあとの処理が失敗したとき(コードは、もう使い切られている)", () => {
  afterEach(() => vi.restoreAllMocks());

  const showFromMemory = () =>
    showAt("/auth/google/complete?code=one-time-code");

  it("ログインの状態の保存が例外になっても、「Try again」は出さず(使用済みのコードで、「リンクが無効」に行き着くため)、失敗の案内を出す", async () => {
    exchangeGoogleCode.mockResolvedValue(signedIn("/shops"));
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota exceeded", "QuotaExceededError");
    });
    const page = await showFromMemory();

    await eventually(() => expect(page.textContent).toContain("Google sign-in failed. Please try again."));

    expect(byText(page, "button", "Try again")).toBeUndefined();
    expect(exchangeGoogleCode).toHaveBeenCalledTimes(1);
  });

  it("成功の応答の本文が、想定した形でない(空の文字列など)ときも、「Try again」は出さない", async () => {
    exchangeGoogleCode.mockResolvedValue("" as unknown as GoogleExchangeResponse);
    const page = await showFromMemory();

    await eventually(() => expect(page.textContent).toContain("Google sign-in failed. Please try again."));

    expect(byText(page, "button", "Try again")).toBeUndefined();
  });
});
