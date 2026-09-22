// @vitest-environment jsdom
import { vi } from "vitest";
vi.hoisted(() => vi.stubEnv("VITE_API_BASE_URL", "/api"));
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { SWRConfig } from "swr";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need } from "../../../test/dom";
import { authApi } from "../api/authApiClient";
import { GoogleConnection } from "./GoogleConnection";

vi.mock("../api/authApiClient", () => ({
  authApi: { listIdentities: vi.fn(), unlinkGoogle: vi.fn(), startGoogleLink: vi.fn() },
}));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { loginProviders: ["google"] } }) }));

const listIdentities = vi.mocked(authApi.listIdentities);
const connected = { identities: [{ provider: "google", email: "carol@gmail.example", connectedAt: "2026-09-21T00:00:00Z", canUnlink: true }] };

const show = () =>
  mount(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}>
      <GoogleConnection viewerId="7" navigateTo={vi.fn()} />
    </SWRConfig>,
  );
const alertText = (page: HTMLElement) => page.querySelector('[role="alert"]')?.textContent ?? "";

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(window, "confirm").mockReturnValue(true);
});
afterEach(cleanup);

describe("GoogleConnection の失敗の表示(見出し + API の文言)", () => {
  it("一覧を取得できなかったとき: 画面の見出しと、失敗の文言を、1 つのエラーにして出す", async () => {
    listIdentities.mockRejectedValue(new ApiError(["boom"], 500));
    const page = await show();

    await eventually(() => expect(alertText(page)).toContain("Failed to load your Google connection."));
    expect(alertText(page)).toContain("Could not load your Google connection");
  });

  it("解除に失敗したとき: 「解除できなかった」の見出しと、API の文言を出す", async () => {
    listIdentities.mockResolvedValue(connected);
    vi.mocked(authApi.unlinkGoogle).mockRejectedValue(new ApiError(["Google is your only way to sign in."], 422));
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Disconnect")).toBeDefined());

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(alertText(page)).toContain("Google is your only way to sign in."));
    expect(alertText(page)).toContain("Could not disconnect Google");
  });

  it("連携を始められなかったとき: 「始められなかった」の見出しと、API の文言を出す", async () => {
    listIdentities.mockResolvedValue({ identities: [] });
    vi.mocked(authApi.startGoogleLink).mockRejectedValue(new ApiError(["Please sign in again."], 401));
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Connect Google")).toBeDefined());

    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));

    await eventually(() => expect(alertText(page)).toContain("Please sign in again."));
    expect(alertText(page)).toContain("Could not start connecting Google");
  });

  it("連携している欄には、「Google account」の見出しと、連携先のメールを出す", async () => {
    listIdentities.mockResolvedValue(connected);
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));
    expect(page.textContent).toContain("Google account");
  });
});
