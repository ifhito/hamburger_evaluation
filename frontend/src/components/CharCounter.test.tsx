import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import "../lib/i18n";
import { CharCounter } from "./CharCounter";
import { Input } from "./Input";
import { Textarea } from "./Textarea";

describe("CharCounter", () => {
  const render = (value: string, max: number | undefined) =>
    renderToStaticMarkup(<CharCounter id="c" value={value} max={max} />);

  it("上限に収まっているときは「文字数 / 上限」だけを見せ、超過の表示を出さない", () => {
    const html = render("あ🍔", 5);
    expect(html).toContain("2 / 5");
    expect(html).not.toContain("Too long");
    expect(html).toContain("Within the limit");
  });

  it("上限ちょうどは超過とみなさない", () => {
    const html = render("ああああああ".slice(0, 5), 5);
    expect(html).toContain("5 / 5");
    expect(html).not.toContain("Too long");
  });

  it("上限を 1 文字超えると、色だけでなく文字(Too long)でも知らせ、読み上げ用の文言も超過に変わる", () => {
    const html = render("あいうえおか", 5);
    expect(html).toContain("6 / 5");
    expect(html).toContain("Too long");
    expect(html).toContain("Over the limit (maximum 5 characters)");
    expect(html).not.toContain("Within the limit");
  });

  it("上限が取得できていない(undefined)ときは、何も出さない(値を推測しない)", () => {
    expect(render("あいう", undefined)).toBe("");
  });

  it("読み上げの領域は、文字数が変わるたびに変わらない(超えた/収まったの 2 つの文言だけ)", () => {
    const within1 = render("a", 5);
    const within2 = render("abcd", 5);
    const status = (html: string) => /<span role="status"[^>]*>(.*?)<\/span>/.exec(html)?.[1];
    expect(status(within1)).toBe(status(within2));
  });
});

describe("Input と Textarea のカウンター", () => {
  it("Input は、上限が取れているときだけカウンターを出し、入力欄に説明として結び付ける", () => {
    const shown = renderToStaticMarkup(<Input id="name" label="Name" counter={{ value: "abc", max: 100 }} />);
    expect(shown).toContain('id="name-counter"');
    expect(shown).toContain('aria-describedby="name-counter"');

    const hidden = renderToStaticMarkup(<Input id="name" label="Name" counter={{ value: "abc", max: undefined }} />);
    expect(hidden).not.toContain("name-counter");
    expect(hidden).not.toContain("aria-describedby");
  });

  it("Input は、説明文とカウンターの両方を aria-describedby に並べる", () => {
    const html = renderToStaticMarkup(
      <Input id="name" label="Name" hint="ヒント" counter={{ value: "", max: 10 }} />,
    );
    expect(html).toContain('aria-describedby="name-hint name-counter"');
  });

  it("Textarea も、上限が取れているときだけカウンターを出す", () => {
    const shown = renderToStaticMarkup(<Textarea id="bio" label="Bio" counter={{ value: "あ", max: 10 }} />);
    expect(shown).toContain("1 / 10");
    expect(shown).toContain('aria-describedby="bio-counter"');

    const hidden = renderToStaticMarkup(<Textarea id="bio" label="Bio" counter={{ value: "あ", max: undefined }} />);
    expect(hidden).not.toContain("bio-counter");
  });

  it("カウンターを渡さない入力欄は、これまでと同じ見た目(カウンターなし)のまま", () => {
    const html = renderToStaticMarkup(<Input id="email" label="Email" />);
    expect(html).not.toContain("counter");
  });
});
