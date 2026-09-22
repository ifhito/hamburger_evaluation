// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import "../../../lib/i18n";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import { copyToClipboard } from "../../../lib/clipboard";
import { ShareLinkButton } from "./ShareLinkButton";

vi.mock("../../../lib/clipboard", () => ({ copyToClipboard: vi.fn() }));
const copy = vi.mocked(copyToClipboard);

beforeEach(() => copy.mockReset());
afterEach(cleanup);

describe("ShareLinkButton(プロフィールのリンクのコピー)", () => {
  it("コピーできたとき: ボタンを押す → 共有用のリンクをコピーし、ボタンの文言が「Link copied」に変わる(入力欄は出さない)", async () => {
    copy.mockResolvedValue(true);
    const page = await mount(<ShareLinkButton userId="7" />);

    await click(need(byText(page, "button", "Copy link"), "Copy link"));

    expect(copy).toHaveBeenCalledWith(`${window.location.origin}/users/7`);
    expect(byText(page, "button", "Link copied")).toBeDefined();
    expect(page.querySelector("input")).toBeNull();
  });

  it("コピーできなかったとき: ボタンを押す → 選んでコピーできる読み取り専用の入力欄に、リンクを出す", async () => {
    copy.mockResolvedValue(false);
    const page = await mount(<ShareLinkButton userId="7" />);

    await click(need(byText(page, "button", "Copy link"), "Copy link"));

    const input = need(page.querySelector("input"), "input");
    expect(input.readOnly).toBe(true);
    expect(input.value).toBe(`${window.location.origin}/users/7`);
    expect(page.textContent).toContain("Copying failed.");
    expect(byText(page, "button", "Link copied")).toBeUndefined();
  });
});
