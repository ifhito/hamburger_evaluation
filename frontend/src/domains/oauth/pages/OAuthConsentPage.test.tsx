// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need, unmount } from "../../../test/dom";
import { oauthApi } from "../api/oauthApiClient";
import type { AuthorizeRequestView } from "../api/types";
import OAuthConsentPage from "./OAuthConsentPage";

vi.mock("../api/oauthApiClient", () => ({ oauthApi: { describe: vi.fn(), decide: vi.fn() } }));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: { id: "9", username: "alice" } }) }));

const describe_ = vi.mocked(oauthApi.describe);
const decide = vi.mocked(oauthApi.decide);

const view: AuthorizeRequestView = {
  client: { id: "claude-code", name: "Claude Code" },
  scopes: [
    { name: "hamburger:read", description: "View shops, reviews and profiles", writes: false },
    { name: "hamburger:write", description: "Post, edit and delete reviews, and submit shops, on your behalf", writes: true },
  ],
  consentRequired: true,
};

const show = (search = "?client_id=claude-code&redirect_uri=http://localhost:53412/callback") =>
  mount(
    <MemoryRouter initialEntries={[`/oauth/authorize${search}`]}>
      <Routes>
        <Route path="/oauth/authorize" element={<OAuthConsentPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

describe("OAuthConsentPage(アプリの接続の許可)", () => {
  it("アプリの名前・ID・範囲の説明を、API が返したとおりに出す。書き込みの範囲だけに印(Writes)を付ける", async () => {
    describe_.mockResolvedValue(view);
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("Claude Code"));
    expect(page.textContent).toContain("App ID: claude-code");
    expect(page.textContent).toContain("View shops, reviews and profiles");
    expect(page.textContent).toContain("Post, edit and delete reviews, and submit shops, on your behalf");
    const items = [...page.querySelectorAll('[class*="scopeItem"]')];
    expect(items[0].textContent).not.toContain("Writes");
    expect(items[1].textContent).toContain("Writes");
  });

  it("戻り先のホストと、サインイン中の名前を出す", async () => {
    describe_.mockResolvedValue(view);
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("localhost:53412"));
    expect(page.textContent).toContain("Signed in as:");
    expect(page.textContent).toContain("alice");
  });

  it("アプリの名前に HTML があっても、文字として出す(解釈しない)", async () => {
    describe_.mockResolvedValue({ ...view, client: { id: "x", name: "<img src=x onerror=alert(1)>" } });
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("<img src=x onerror=alert(1)>"));
    expect(page.querySelector("img")).toBeNull();
  });

  it("「Allow」を押すと、いま見せている要求を許可で送り、返された戻り先へ移動する(navigate は window.location で確かめない範囲)", async () => {
    describe_.mockResolvedValue(view);
    decide.mockResolvedValue({ redirectTo: "http://localhost:53412/callback?code=abc" });
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Allow")).toBeDefined());

    await click(need(byText(page, "button", "Allow"), "Allow"));

    expect(decide).toHaveBeenCalledWith("?client_id=claude-code&redirect_uri=http://localhost:53412/callback", true);
    await eventually(() => expect(page.textContent).toContain("Connecting"));
  });

  it("「Deny」を押すと、拒否で送る", async () => {
    describe_.mockResolvedValue(view);
    decide.mockResolvedValue({ redirectTo: "http://localhost:53412/callback?error=access_denied" });
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Deny")).toBeDefined());

    await click(need(byText(page, "button", "Deny"), "Deny"));

    expect(decide).toHaveBeenCalledWith(expect.any(String), false);
  });

  it("取得に失敗したとき: 画面の見出しと、API の文言を出す", async () => {
    describe_.mockRejectedValue(new ApiError(["oauth authorization request is invalid"], 422));
    const page = await show();

    await eventually(() => expect(page.querySelector('[role="alert"]')?.textContent).toContain("oauth authorization request is invalid"));
    expect(page.textContent).toContain("Could not connect");
    expect(page.textContent).toContain("Go back to the app and try again.");
  });

  it("同意が要らない要求(consentRequired が false)のときは、尋ねずにそのまま許可を送り、戻り先へ移動する", async () => {
    describe_.mockResolvedValue({ ...view, consentRequired: false });
    decide.mockResolvedValue({ redirectTo: "http://localhost:53412/callback?code=abc" });
    await show();

    await eventually(() => expect(decide).toHaveBeenCalledWith(expect.any(String), true, expect.anything()));
  });

  it("「Allow」を押したあとに画面を離れたら、応答が返っても、そのブラウザを勝手に移動させない", async () => {
    describe_.mockResolvedValue(view);
    let resolveDecide!: (v: { redirectTo: string }) => void;
    decide.mockReturnValue(new Promise((resolve) => (resolveDecide = resolve)));
    // jsdom の window.location.assign は個別に差し替えられないので、location ごと差し替える。
    const originalLocation = window.location;
    const assign = vi.fn();
    Object.defineProperty(window, "location", { value: { ...originalLocation, assign }, writable: true, configurable: true });
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Allow")).toBeDefined());
    await click(need(byText(page, "button", "Allow"), "Allow"));

    await unmount(page);
    resolveDecide({ redirectTo: "http://localhost:53412/callback?code=abc" });
    await eventually(() => expect(decide).toHaveBeenCalled());

    expect(assign).not.toHaveBeenCalled();
    Object.defineProperty(window, "location", { value: originalLocation, writable: true, configurable: true });
  });
});
