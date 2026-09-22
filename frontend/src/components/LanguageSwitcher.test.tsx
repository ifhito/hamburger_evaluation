// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import i18n from "../lib/i18n";
import { cleanup, click, eventually, mount } from "../test/dom";
import { LanguageSwitcher } from "./LanguageSwitcher";

describe("LanguageSwitcher(ヘッダーの JA / EN。design/redesign/mock.css の .lang / .lang-btn と同じ要素)", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en");
  });
  afterEach(async () => {
    await cleanup();
    await i18n.changeLanguage("en");
  });

  it("button 2 つ(JA・EN)で、いまの言語の方に選択中の印が付く", async () => {
    const page = await mount(<LanguageSwitcher />);
    const buttons = [...page.querySelectorAll("button")];
    expect(buttons.map((b) => b.textContent)).toEqual(["JA", "EN"]);

    const [ja, en] = buttons;
    expect(en.getAttribute("aria-pressed")).toBe("true");
    expect(ja.getAttribute("aria-pressed")).toBe("false");
  });

  it("押すと、i18n の言語が切り替わり、選択中の印も動く(AC2)", async () => {
    const page = await mount(<LanguageSwitcher />);
    const [ja] = [...page.querySelectorAll("button")];

    await click(ja);

    await eventually(() => expect(ja.getAttribute("aria-pressed")).toBe("true"));
    expect(i18n.language).toBe("ja");
  });
});
