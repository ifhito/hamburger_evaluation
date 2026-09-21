// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
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
// GET /meta の内容は、テストごとに差し替える(既定は、Google が有効)。
const meta = vi.hoisted(() => ({ loginProviders: ["google"] as string[] }));
vi.mock("../../../api/meta", () => ({ useMeta: () => ({ data: meta }) }));

const listIdentities = vi.mocked(authApi.listIdentities);
const unlinkGoogle = vi.mocked(authApi.unlinkGoogle);
const startGoogleLink = vi.mocked(authApi.startGoogleLink);

const connected: IdentitiesResponse = {
  identities: [{ provider: "google", email: "carol@gmail.example", connectedAt: "2026-09-21T00:00:00Z", canUnlink: true } satisfies Identity],
};
const notConnected: IdentitiesResponse = { identities: [] };

// 呼び出しごとに新しいキャッシュにして、テスト同士で結果を持ち越さない(再試行もしない)。
const navigateTo = vi.fn<(url: string) => void>();
const show = () =>
  mount(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}>
      <GoogleConnection viewerId="7" navigateTo={navigateTo} />
    </SWRConfig>,
  );

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(window, "confirm").mockReturnValue(true);
  meta.loginProviders = ["google"];
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

  it("「結び付ける」を押すと、認証つきの開始を、このプロフィールの戻り先で呼び、返された Google の URL へ移動する", async () => {
    listIdentities.mockResolvedValue(notConnected);
    startGoogleLink.mockResolvedValue({ redirectUrl: "https://accounts.google.com/o/oauth2/v2/auth?state=s" });
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Connect Google")).toBeDefined());

    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));

    expect(startGoogleLink).toHaveBeenCalledWith("/users/7");
    expect(navigateTo).toHaveBeenCalledWith("https://accounts.google.com/o/oauth2/v2/auth?state=s");
  });

  it("開始が返した URL が http(s) でないとき(javascript: など)は、移動せず、エラーを出し、ボタンを押せる状態に戻す", async () => {
    listIdentities.mockResolvedValue(notConnected);
    startGoogleLink.mockResolvedValue({ redirectUrl: "javascript:alert(1)" });
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Connect Google")).toBeDefined());

    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));

    expect(navigateTo).not.toHaveBeenCalled();
    await eventually(() => expect(page.textContent).toContain("Failed to start connecting Google."));
    expect(need(byText(page, "button", "Connect Google"), "Connect Google").hasAttribute("disabled")).toBe(false);
  });

  it("開始がサーバーの文言のエラーで失敗したときは、その文言を出し、移動しない", async () => {
    listIdentities.mockResolvedValue(notConnected);
    startGoogleLink.mockRejectedValue(new ApiError(["Please sign in again."], 401));
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Connect Google")).toBeDefined());

    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));

    await eventually(() => expect(page.textContent).toContain("Please sign in again."));
    expect(navigateTo).not.toHaveBeenCalled();
  });

  it("Google の画面から戻る(bfcache で、画面の状態ごと復元される)と、処理中のままにならず、もう一度「結び付ける」を押せる", async () => {
    listIdentities.mockResolvedValue(notConnected);
    startGoogleLink.mockResolvedValue({ redirectUrl: "https://accounts.google.com/o/oauth2/v2/auth?state=s" });
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Connect Google")).toBeDefined());
    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));
    expect(page.textContent).toContain("Loading"); // 移動している間は、処理中

    await act(async () => {
      window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }));
    });

    expect(need(byText(page, "button", "Connect Google"), "Connect Google").hasAttribute("disabled")).toBe(false);
  });

  it("ふつうの読み込み(bfcache でない pageshow)では、処理中の表示を変えない", async () => {
    listIdentities.mockResolvedValue(notConnected);
    startGoogleLink.mockReturnValue(new Promise(() => undefined));
    const page = await show();
    await eventually(() => expect(byText(page, "button", "Connect Google")).toBeDefined());
    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));

    await act(async () => {
      window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: false }));
    });

    expect(page.textContent).toContain("Loading");
  });

  it("「Google disconnected.」の知らせは、次の操作(結び付ける)を始めたら消える(失敗の文言と同時に出ない)", async () => {
    listIdentities.mockResolvedValueOnce(connected).mockResolvedValue(notConnected);
    unlinkGoogle.mockResolvedValue(undefined);
    startGoogleLink.mockRejectedValue(new ApiError(["Cannot start now."], 500));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));
    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));
    await eventually(() => expect(page.textContent).toContain("Google disconnected."));

    await click(need(byText(page, "button", "Connect Google"), "Connect Google"));

    await eventually(() => expect(page.textContent).toContain("Cannot start now."));
    expect(page.textContent).not.toContain("Google disconnected.");
  });

  it("別のタブなどで、すでに解除済み(解除が 404)のときは、解除できたものとして扱い、キャッシュから外して、再取得する(生の「not found」は出さない)", async () => {
    listIdentities.mockResolvedValueOnce(connected).mockResolvedValue(notConnected);
    unlinkGoogle.mockRejectedValue(new ApiError(["not found"], 404));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("Google disconnected."));
    expect(page.textContent).not.toContain("not found");
    expect(page.textContent).not.toContain("Connected as");
    expect(byText(page, "button", "Connect Google")).toBeDefined();
    await eventually(() => expect(listIdentities).toHaveBeenCalledTimes(2));
  });

  it("解除の失敗が 404 以外(422・500 など)のときは、これまでどおり、連携の表示を残して、失敗を出す", async () => {
    listIdentities.mockResolvedValue(connected);
    unlinkGoogle.mockRejectedValue(new ApiError(["internal server error"], 500));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("internal server error"));
    expect(page.textContent).toContain("Connected as carol@gmail.example");
    expect(page.textContent).not.toContain("Google disconnected.");
  });

  it("結び付いていて、backend が解除できないと返したとき(サインインする方法がなくなる)は、「解除」を出さず、理由を出す", async () => {
    listIdentities.mockResolvedValue({ identities: [{ ...connected.identities[0], canUnlink: false }] });
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    expect(byText(page, "button", "Disconnect")).toBeUndefined();
    expect(page.textContent).toContain("Add a password");
  });

  it("メールに HTML があっても、文字として描画する(解釈しない)", async () => {
    listIdentities.mockResolvedValue({ identities: [{ ...connected.identities[0], email: "<img src=x onerror=alert(1)>@evil.example" }] });
    const page = await show();

    await eventually(() => expect(page.textContent).toContain("Connected as <img src=x onerror=alert(1)>@evil.example"));

    expect(page.querySelector("img")).toBeNull();
  });

  it("解除の通信中は、処理中の表示になり、連携の表示は、そのまま", async () => {
    listIdentities.mockResolvedValue(connected);
    unlinkGoogle.mockReturnValue(new Promise(() => undefined));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    expect(page.textContent).toContain("Loading");
    expect(page.textContent).toContain("Connected as carol@gmail.example");
  });

  it("取得に失敗し続けても、自動の再取得で、「失敗と Retry」が「読み込み中」に切り替わらない(再取得は、Retry を押したときだけ)", async () => {
    listIdentities.mockRejectedValue(new ApiError(["boom"], 500));
    // SWR の既定(失敗すると、間隔をあけて自動で再取得する)のまま。間隔だけを短くする。
    const page = await mount(
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, errorRetryInterval: 15 }}>
        <GoogleConnection viewerId="7" navigateTo={navigateTo} />
      </SWRConfig>,
    );
    await eventually(() => expect(byText(page, "button", "Retry")).toBeDefined());

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 400));
    });

    expect(listIdentities).toHaveBeenCalledTimes(1);
    expect(byText(page, "button", "Retry")).toBeDefined();
    expect(page.textContent).not.toContain("Loading");
  });

  it("GET /meta が Google を返さない(または、この項目がない)ときは、何も出さず、一覧も取得しない", async () => {
    for (const providers of [[], ["other"], undefined]) {
      meta.loginProviders = providers as unknown as string[];
      listIdentities.mockResolvedValue(connected);
      const page = await show();

      expect(page.textContent).toBe("");
      expect(listIdentities).not.toHaveBeenCalled();
      await cleanup();
    }
  });

  it("解除が 404 のとき、取り直した一覧に、まだ Google の連携があれば(解除済みではない)、解除できたとは言わず、失敗を出す", async () => {
    listIdentities.mockResolvedValue(connected); // 取り直しても、連携が残っている
    unlinkGoogle.mockRejectedValue(new ApiError(["not found"], 404));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("Failed to disconnect Google."));
    expect(page.textContent).not.toContain("Google disconnected.");
    expect(page.textContent).toContain("Connected as carol@gmail.example");
  });

  it("解除が 404 のとき、一覧を取り直せなかったときも、解除できたとは言わず、失敗を出す(Google の機能が止まっているときの 404 を、解除済みと取り違えない)", async () => {
    listIdentities.mockResolvedValueOnce(connected).mockRejectedValue(new ApiError(["not found"], 404));
    unlinkGoogle.mockRejectedValue(new ApiError(["not found"], 404));
    const page = await show();
    await eventually(() => expect(page.textContent).toContain("Connected as carol@gmail.example"));

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    await eventually(() => expect(page.textContent).toContain("Failed to disconnect Google."));
    expect(page.textContent).not.toContain("Google disconnected.");
  });
});
