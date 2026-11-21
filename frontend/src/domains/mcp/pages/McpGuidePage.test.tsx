// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import i18n from "../../../lib/i18n";
import { returnPathFrom } from "../../../app/router/returnTo";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import { copyToClipboard } from "../../../lib/clipboard";
import McpGuidePage from "./McpGuidePage";

const authState = vi.hoisted(() => ({ user: null as { id: string; username: string } | null }));
vi.mock("../../../lib/clipboard", () => ({ copyToClipboard: vi.fn() }));
vi.mock("../../auth/AuthProvider", () => ({ useAuth: () => ({ user: authState.user, isLoading: false }) }));
const copy = vi.mocked(copyToClipboard);
beforeEach(async () => { authState.user = null; copy.mockReset(); await i18n.changeLanguage("en"); });
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

  it("未サインイン時の解除案内は、サインイン後にマイページ(/me)へ戻れるよう state.from を渡す", async () => {
    function SigninDest() {
      const loc = useLocation();
      return <p>from:{returnPathFrom(loc.state) ?? "none"}</p>;
    }
    const page = await mount(
      <MemoryRouter initialEntries={["/mcp"]}>
        <Routes>
          <Route path="/mcp" element={<McpGuidePage />} />
          <Route path="/signin" element={<SigninDest />} />
        </Routes>
      </MemoryRouter>,
    );
    await click(need(byText(page, "a", "Sign in"), "サインインリンク"));
    expect(page.textContent).toBe("from:/me");
  });

  it("サインイン済みのときは、新規登録ボタンを出さず、解除案内から本人のプロフィールへ直接案内する", async () => {
    authState.user = { id: "u-42", username: "alice" };
    const page = await render();
    expect(page.querySelector('a[href="/signup"]')).toBeNull();
    expect(page.querySelector('a[href="/users/u-42"]')?.textContent).toBe("My page");
  });
});
