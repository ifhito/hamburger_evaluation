import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Logo, SMALL_MARK_BELOW_PX } from "./Logo";
import { MARK, MARK_SMALL } from "./logoMarks";

const render = (props: Parameters<typeof Logo>[0]) => renderToStaticMarkup(<Logo {...props} />);

describe("Logo の記号の版", () => {
  it("高さが 24px 以上のときは、レタスの波つきの通常版を使う", () => {
    expect(SMALL_MARK_BELOW_PX).toBe(24);
    const html = render({ name: "BurgerStack", size: 24 });
    expect(html).toContain('data-mark="normal"');
    expect(html).toContain(`d="${MARK.path}"`);
  });

  it("高さが 24px 未満のときは、波を省いた小さい版に切り替える", () => {
    const html = render({ name: "BurgerStack", size: 23 });
    expect(html).toContain('data-mark="small"');
    expect(html).toContain(`d="${MARK_SMALL.path}"`);
    expect(html).not.toContain(`d="${MARK.path}"`);
  });

  it("大きさを指定しないときは、ヘッダーの高さ(32px)の通常版になる", () => {
    const html = render({ name: "BurgerStack" });
    expect(html).toContain('height="32"');
    expect(html).toContain('data-mark="normal"');
  });

  it("通常版と小さい版とで、線の太さが違う(小さい版は太い)", () => {
    expect(MARK_SMALL.strokeWidth).toBeGreaterThan(MARK.strokeWidth);
    expect(render({ name: "BurgerStack", size: 32 })).toContain(`stroke-width="${MARK.strokeWidth}"`);
    expect(render({ name: "BurgerStack", size: 16 })).toContain(`stroke-width="${MARK_SMALL.strokeWidth}"`);
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
