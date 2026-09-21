// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need } from "../../../test/dom";
import { authApi } from "../api/authApiClient";
import type { Identity, IdentitiesResponse } from "../types";
import { GoogleConnection } from "./GoogleConnection";

vi.mock("../api/authApiClient", () => ({
  authApi: { listIdentities: vi.fn(), unlinkGoogle: vi.fn(), startGoogleLink: vi.fn() },
}));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: { loginProviders: ["google"] } }) }));

const listIdentities = vi.mocked(authApi.listIdentities);
const unlinkGoogle = vi.mocked(authApi.unlinkGoogle);

const connected: IdentitiesResponse = {
  identities: [{ provider: "google", email: "carol@gmail.example", connectedAt: "2026-09-21T00:00:00Z", canUnlink: true } satisfies Identity],
};
const notConnected: IdentitiesResponse = { identities: [] };

// 呼び出しごとに新しいキャッシュにして、テスト同士で結果を持ち越さない(再試行もしない)。
const show = () =>
  mount(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}>
      <GoogleConnection viewerId="7" />
    </SWRConfig>,
  );

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(window, "confirm").mockReturnValue(true);
});
afterEach(cleanup);

describe("GoogleConnection(プロフィールの Google の連携。一覧の取得と解除)", () => {
  it("一覧の取得に失敗したときは、エラーと再試行だけを出し、「未連携」の表示と「結び付ける」の操作は出さない", async () => {
    listIdentities.mockRejectedValue(new ApiError(["boom"], 500));
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("Failed to load your Google connection."));
    expect(page.textContent).not.toContain("Not connected");
    expect(byText(page, "button", "Connect Google")).toBeUndefined();
    expect(byText(page, "button", "Retry")).toBeDefined();
  });

  it("再試行して一覧を取得できたら、そのときの連携の状態と操作を出す", async () => {
    listIdentities.mockRejectedValueOnce(new ApiError(["boom"], 500)).mockResolvedValueOnce(connected);
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Retry")).toBeDefined());

    await click(need(byText(page, "button", "Retry"), "Retry"));

    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));
    expect(page.textContent).not.toContain("Failed to load your Google connection.");
    expect(byText(page, "button", "Disconnect")).toBeDefined();
    expect(listIdentities).toHaveBeenCalledTimes(2);
  });

  it("一覧の取得中は、「未連携」とは表示せず、読み込み中と出す", async () => {
    listIdentities.mockReturnValue(new Promise(() => undefined));
    const page = await show();

    expect(page.textContent).toContain("Loading");
    expect(page.textContent).not.toContain("Not connected");
    expect(byText(page, "button", "Connect Google")).toBeUndefined();
  });

  it("一覧を取得できて、連携がなければ、「未連携」と「結び付ける」を出す", async () => {
    listIdentities.mockResolvedValue(notConnected);
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("Not connected."));
    expect(byText(page, "button", "Connect Google")).toBeDefined();
  });

  it("解除が成功したあとに、一覧の再取得だけが失敗しても、解除できたと出し、古い連携の表示は残さない", async () => {
    listIdentities.mockResolvedValueOnce(connected).mockRejectedValue(new ApiError(["boom"], 500));
    unlinkGoogle.mockResolvedValue(undefined);
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("Google disconnected."));
    expect(unlinkGoogle).toHaveBeenCalledTimes(1);
    expect(page.textContent).not.toContain("Connected as");
    expect(byText(page, "button", "Disconnect")).toBeUndefined();
    expect(page.textContent).not.toContain("Failed to disconnect Google.");
    expect(page.textContent).toContain("Not connected.");
  });

  it("解除が成功して、再取得も成功したときも、解除できたと出し、未連携の表示になる", async () => {
    listIdentities.mockResolvedValueOnce(connected).mockResolvedValue(notConnected);
    unlinkGoogle.mockResolvedValue(undefined);
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("Google disconnected."));
    expect(page.textContent).toContain("Not connected.");
    expect(byText(page, "button", "Connect Google")).toBeDefined();
    // サーバーの状態を、あらためて取り直している(表示の書き換えだけで済ませない)。
    await eventually(() => expect(listIdentities).toHaveBeenCalledTimes(2));
  });

  it("解除が失敗したとき(サーバーの文言)は、連携の表示を残したまま、その文言を出す", async () => {
    listIdentities.mockResolvedValue(connected);
    unlinkGoogle.mockRejectedValue(new ApiError(["Google is your only way to sign in."], 422));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("Google is your only way to sign in."));
    expect(page.textContent).toContain("Connected as carol@gmail.example");
    expect(page.textContent).not.toContain("Google disconnected.");
  });
});
