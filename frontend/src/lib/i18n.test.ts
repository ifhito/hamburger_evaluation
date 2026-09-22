// @vitest-environment jsdom
// lib/i18n.ts は、読み込まれた瞬間の navigator.language / localStorage で、既定の言語を決める(モジュールの
// 先頭で実行される副作用)。シナリオごとに違う条件で確かめるため、毎回 vi.resetModules() してから読み直す。
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

function setNavigatorLanguages(...languages: string[]): void {
  Object.defineProperty(window.navigator, "language", { value: languages[0], configurable: true });
  Object.defineProperty(window.navigator, "languages", { value: languages, configurable: true });
}

async function loadI18n() {
  vi.resetModules();
  return import("./i18n");
}

describe("lib/i18n の既定の言語(AC1・AC4・AC6)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("ブラウザの言語が日本語のときは、既定で日本語になり、<html lang> も日本語になる", async () => {
    setNavigatorLanguages("ja-JP");
    const { default: i18n } = await loadI18n();
    expect(i18n.language).toBe("ja");
    expect(document.documentElement.lang).toBe("ja");
  });

  it("ブラウザの言語が英語のときは、既定で英語になる", async () => {
    setNavigatorLanguages("en-US");
    const { default: i18n } = await loadI18n();
    expect(i18n.language).toBe("en");
    expect(document.documentElement.lang).toBe("en");
  });

  it("対応外の言語(例: フランス語)のときは、英語になる", async () => {
    setNavigatorLanguages("fr-FR");
    const { default: i18n } = await loadI18n();
    expect(i18n.language).toBe("en");
  });

  it("保存した言語があれば、ブラウザの言語より優先する(AC2)", async () => {
    setNavigatorLanguages("ja-JP");
    localStorage.setItem("burgerstack:lang", "en");
    const { default: i18n } = await loadI18n();
    expect(i18n.language).toBe("en");
  });

  it("localStorage が使えない(getItem が例外を投げる)環境でも、落ちずにブラウザの言語になる(AC4)", async () => {
    setNavigatorLanguages("ja-JP");
    const spy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked in private mode");
    });
    try {
      const { default: i18n } = await loadI18n();
      expect(i18n.language).toBe("ja");
    } finally {
      spy.mockRestore();
    }
  });
});

describe("lib/i18n の切り替え(AC2・AC6)", () => {
  beforeEach(() => {
    localStorage.clear();
    setNavigatorLanguages("en-US");
  });
  afterEach(() => vi.restoreAllMocks());

  it("changeLanguage で切り替えると、<html lang> が変わり、次回のために保存される", async () => {
    const { default: i18n } = await loadI18n();
    await i18n.changeLanguage("ja");
    expect(document.documentElement.lang).toBe("ja");
    expect(localStorage.getItem("burgerstack:lang")).toBe("ja");

    const { default: reloaded } = await loadI18n();
    expect(reloaded.language).toBe("ja");
  });

  it("保存に失敗しても(setItem が例外)、切り替え自体は落ちない(AC4)", async () => {
    const { default: i18n } = await loadI18n();
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota exceeded");
    });
    await i18n.changeLanguage("ja"); // 例外を投げずに終わることを確かめる(投げれば、このテストが失敗する)
    expect(document.documentElement.lang).toBe("ja");
  });
});
