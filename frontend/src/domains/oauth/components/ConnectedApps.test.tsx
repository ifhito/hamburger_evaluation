// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import "../../../lib/i18n";
import { ApiError } from "../../../api/client/buildApiClient";
import { byText, cleanup, click, eventually, mount, need } from "../../../test/dom";
import type { ConnectedApp } from "../api/types";
import { ConnectedApps } from "./ConnectedApps";

const state = vi.hoisted(() => ({
  data: undefined as ConnectedApp[] | undefined,
  error: undefined as unknown,
  isLoading: false,
  isDisabled: false,
  hasNextPage: false,
}));
const revoke = vi.hoisted(() => vi.fn());
vi.mock("../hooks/useConnectedApps", () => ({
  useConnectedApps: () => ({
    data: state.data,
    error: state.error,
    isLoading: state.isLoading,
    isDisabled: state.isDisabled,
    hasNextPage: state.hasNextPage,
    fetchNextPage: vi.fn(),
    isFetchingNextPage: false,
    revoke,
  }),
}));

const app = (over: Partial<ConnectedApp>): ConnectedApp => ({
  id: "g1",
  clientId: "claude-code",
  clientName: "Claude Code",
  scopes: [
    { name: "hamburger:read", description: "Read shops, reviews and profiles", writes: false },
    { name: "hamburger:write", description: "Post reviews on your behalf", writes: true },
  ],
  createdAt: "2026-09-21T00:00:00Z",
  updatedAt: "2026-09-21T00:00:00Z",
  ...over,
});

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(window, "confirm").mockReturnValue(true);
  state.data = undefined;
  state.error = undefined;
  state.isLoading = false;
  state.isDisabled = false;
  state.hasNextPage = false;
});
afterEach(cleanup);

describe("ConnectedApps(許可した AI アプリの一覧)", () => {
  it("認可サーバーが無効(404)なときは、何も出さない", async () => {
    state.isDisabled = true;
    const page = await mount(<ConnectedApps viewerId="9" />);

    expect(page.textContent).toBe("");
  });

  it("書き込みの範囲だけに Writes の印を付け、それ以外の範囲には付けない", async () => {
    state.data = [app({})];
    const page = await mount(<ConnectedApps viewerId="9" />);

    const items = [...page.querySelectorAll('[class*="scopeItem"]')];
    expect(items[0].textContent).not.toContain("Writes");
    expect(items[1].textContent).toContain("Writes");
  });

  it("取り消しの確認のあと、取り消しを送る。取り消せなかったときは、画面の見出しと API の文言を出す", async () => {
    state.data = [app({})];
    revoke.mockRejectedValue(new ApiError(["boom"], 500));
    const page = await mount(<ConnectedApps viewerId="9" />);

    await click(need(byText(page, "button", "Disconnect"), "Disconnect"));

    expect(revoke).toHaveBeenCalledWith("g1");
    await eventually(() => expect(page.querySelector('[role="alert"]')?.textContent).toContain("boom"));
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Could not disconnect the app");
  });

  it("取得の失敗は、画面の見出しと API の文言を出す", async () => {
    state.error = new ApiError(["Failed to load connected apps."], 500);
    const page = await mount(<ConnectedApps viewerId="9" />);

    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Could not load connected apps");
    expect(page.querySelector('[role="alert"]')?.textContent).toContain("Failed to load connected apps.");
  });

  it("アプリの名前に HTML があっても、文字として出す(解釈しない)", async () => {
    state.data = [app({ clientName: "<img src=x onerror=alert(1)>" })];
    const page = await mount(<ConnectedApps viewerId="9" />);

    expect(page.textContent).toContain("<img src=x onerror=alert(1)>");
    expect(page.querySelector("img")).toBeNull();
  });

  it("0 件のときは「No apps are connected.」を出す", async () => {
    state.data = [];
    const page = await mount(<ConnectedApps viewerId="9" />);

    expect(page.textContent).toContain("No apps are connected.");
  });
});
