// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import i18n from "../../../lib/i18n";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import { copyToClipboard } from "../../../lib/clipboard";
import McpGuidePage from "./McpGuidePage";

vi.mock("../../../lib/clipboard", () => ({ copyToClipboard: vi.fn() }));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: null }) }));
const copy = vi.mocked(copyToClipboard);
beforeEach(async () => { copy.mockReset(); await i18n.changeLanguage("en"); });
afterEach(cleanup);

const render = () => mount(<MemoryRouter><McpGuidePage /></MemoryRouter>);

describe("MCPの接続案内", () => {
  it("URLをコピーすると、本番の接続先を渡して成功を伝える", async () => {
    copy.mockResolvedValue(true);
    const page = await render();
    await click(need(byText(page, "button", "Copy URL"), "コピー"));
    expect(copy).toHaveBeenCalledWith("https://burger-stack.com/api/mcp");
    expect(page.querySelector('[role="status"]')?.textContent).toBe("URL copied");
  });
  it("コピーが拒否されても、手動で選べるURLと案内を残す", async () => {
    copy.mockResolvedValue(false);
    const page = await render();
    await click(need(byText(page, "button", "Copy URL"), "コピー"));
    const input = need(page.querySelector("input"), "接続URL");
    expect(input.readOnly).toBe(true);
    expect(input.value).toBe("https://burger-stack.com/api/mcp");
    expect(page.querySelector('[role="status"]')?.textContent).toContain("Copying failed");
  });
  it("日本語でも、Claudeの設定と一般ユーザー用の権限を説明する", async () => {
    await i18n.changeLanguage("ja");
    const page = await render();
    expect(page.querySelector("h1")?.textContent).toBe("AIからBurger Stackを使う");
    expect(page.textContent).toContain("Claudeでつなぐ");
    expect(page.textContent).toContain("Claude Codeでつなぐ");
    expect(page.textContent).not.toContain("未検証");
    expect(page.textContent).not.toContain("確認済み");
    expect(page.textContent).toContain("hamburger:read,hamburger:write");
    expect(page.textContent).not.toContain("hamburger:admin");
    expect(page.querySelector('footer a[href="/mcp"]')).not.toBeNull();
  });
});
