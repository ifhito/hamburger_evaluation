import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Logo } from "./Logo";
import markSvg from "../assets/logo/burgerstack-mark.svg?raw";

const render = (props: Parameters<typeof Logo>[0]) => renderToStaticMarkup(<Logo {...props} />);
const markPath = /<path d="([^"]+)"/.exec(markSvg)![1];
const markStrokeWidth = /stroke-width="([^"]+)"/.exec(markSvg)![1];

describe("Logo の記号", () => {
  it("どの大きさでも、レタスの波つきの同じ記号(同じ path・同じ線の太さ)を使う", () => {
    for (const size of [16, 20, 23, 24, 32, 64]) {
      const html = render({ name: "BurgerStack", size });
      expect(html, `${size}px`).toContain(`d="${markPath}"`);
      expect(html, `${size}px`).toContain(`stroke-width="${markStrokeWidth}"`);
    }
  });

  it("大きさを指定しないときは、ヘッダーの高さ(32px)になり、幅は記号の縦横の比から決まる", () => {
    const html = render({ name: "BurgerStack" });
    expect(html).toContain('height="32"');
    const [, , w, h] = /viewBox="([^"]+)"/.exec(markSvg)![1].split(" ").map(Number);
    expect(html).toContain(`width="${Math.round((32 * w) / h)}"`);
  });

  it("大きさを変えると、高さと幅が、その大きさに比例して変わる", () => {
    const small = render({ name: "BurgerStack", size: 16 });
    const large = render({ name: "BurgerStack", size: 64 });
    const widthOf = (html: string) => Number(/ width="(\d+)"/.exec(html)![1]);
    expect(small).toContain('height="16"');
    expect(large).toContain('height="64"');
    expect(widthOf(large)).toBeGreaterThan(widthOf(small) * 3);
  });
});

describe("Logo のアクセシビリティと文字", () => {
  it("記号は装飾として読み上げの対象から外し、文字だけがブランド名として読まれる", () => {
    const html = render({ name: "BurgerStack" });
    expect(html).toContain('aria-hidden="true"');
    expect(html).toContain('focusable="false"');
    expect(html.replace(/<svg.*?<\/svg>/, "").replace(/<[^>]+>/g, "")).toBe("BurgerStack");
  });

  it("ブランド名は、大文字の切れ目で、前半を太字にし、後半は標準の太さのままにする", () => {
    const html = render({ name: "BurgerStack" });
    expect(html).toMatch(/<span class="[^"]*first[^"]*">Burger<\/span>Stack/);
  });

  it("大文字の切れ目がない名前は、全体を 1 つの太字として出す", () => {
    const html = render({ name: "Burger" });
    expect(html).toMatch(/<span class="[^"]*first[^"]*">Burger<\/span>/);
    expect(html.replace(/<svg.*?<\/svg>/, "").replace(/<[^>]+>/g, "")).toBe("Burger");
  });
});

describe("Logo の配色", () => {
  it("既定は白い面用で、暗い面用を選んだときだけ、暗い面用の指定が付く", () => {
    expect(render({ name: "BurgerStack" })).not.toContain("onDark");
    expect(render({ name: "BurgerStack", variant: "on-dark" })).toContain("onDark");
  });
});
