import { describe, it, expect } from "vitest";
import indexHtml from "../../index.html?raw";
import i18n from "./i18n";

describe("サービスの名前", () => {
  it("ヘッダーに出るブランド名は「BurgerStack」である", () => {
    expect(i18n.t("nav.brand")).toBe("BurgerStack");
  });

  it("ブラウザのタブに出るページのタイトルは「BurgerStack」である", () => {
    expect(indexHtml).toContain("<title>BurgerStack</title>");
  });
});
