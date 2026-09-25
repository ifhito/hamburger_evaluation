// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { act } from "react";
import i18n from "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import { ScopeList } from "./ScopeList";

afterEach(async () => {
  await cleanup();
  await i18n.changeLanguage("en");
});

describe("許可した範囲の説明", () => {
  it("日本語で説明し、英語への切り替えにも追従する", async () => {
    await i18n.changeLanguage("ja");
    const page = await mount(<ScopeList scopes={[
      { name: "hamburger:read", description: "View shops, reviews and profiles", writes: false },
      { name: "hamburger:write", description: "Write", writes: true },
      { name: "hamburger:admin", description: "Moderate", writes: true },
    ]} />);
    expect(page.textContent).toContain("お店、レビュー、プロフィールを閲覧する");
    expect(page.textContent).toContain("レビューの投稿・編集・削除");
    expect(page.textContent).toContain("閉業・営業再開を登録する");
    await act(async () => { await i18n.changeLanguage("en"); });
    expect(page.textContent).toContain("View shops, reviews and profiles");
    expect(page.textContent).not.toContain("閲覧する");
  });

  it("未知の範囲はサーバーの説明を文字として表示する", async () => {
    await i18n.changeLanguage("ja");
    const page = await mount(<ScopeList scopes={[
      { name: "future:scope", description: "<b>Future permission</b>", writes: false },
      { name: "toString", description: "Unknown scope", writes: false },
    ]} />);
    expect(page.textContent).toContain("<b>Future permission</b>");
    expect(page.textContent).toContain("Unknown scope");
    expect(page.querySelector("b")).toBeNull();
  });

  it("書き込みの印は範囲の名前から推測せずサーバーの値に従う", async () => {
    await i18n.changeLanguage("ja");
    const page = await mount(<ScopeList scopes={[
      { name: "hamburger:write", description: "Write", writes: false },
      { name: "future:scope", description: "Future permission", writes: true },
    ]} />);
    const items = page.querySelectorAll("li");
    expect(items[0].textContent).not.toContain("書き込み");
    expect(items[1].textContent).toContain("書き込み");
  });
});
