import { describe, it, expect } from "vitest";
import globalsCss from "../../app/styles/globals.css?raw";
import wghtCss from "@fontsource-variable/noto-sans-jp/wght.css?raw";
import mainTsx from "../../main.tsx?raw";

// デザインのトークン(リポジトリの design/redesign/mock.css)。frontend だけを取り出した環境では design/ が無いので、
// その場合は、デザインとの一致の確認を飛ばす(CI では全体が取り出されるので、実行される)。
const designCss = import.meta.glob<string>("../../../../design/redesign/mock.css", { query: "?raw", import: "default", eager: true });
const design = Object.values(designCss)[0];

// CI では、デザインのファイルが無い(移動・改名された)ときは、飛ばさず失敗にする(飛ばすのは、frontend だけを取り出した環境のため)。
const inCi = Boolean((globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env.CI);
it.runIf(inCi)("CI では、デザインの見本のファイルが見つかる", () => expect(design).toBeTruthy());

const rootBlock = (css: string) => /:root\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";
const variables = (css: string) => Object.fromEntries([...rootBlock(css).matchAll(/--([\w-]+)\s*:\s*([^;]+);/g)].map(([, name, value]) => [name, value.trim()]));
const own = variables(globalsCss);

it("--ui-font の先頭は、main.tsx が読み込む可変フォントの family 名と一致する(違うと、読み込んだフォントが使われない)", () => {
  const family = /font-family:\s*'([^']+)'/.exec(wghtCss)?.[1];
  expect(family).toBe("Noto Sans JP Variable");
  expect(mainTsx).toContain("@fontsource-variable/noto-sans-jp/wght.css");
  expect(own["ui-font"].startsWith(`"${family}"`)).toBe(true);
});

describe.skipIf(!design)("デザインのトークンとの一致", () => {
  const dv = variables(design);

  it("mock.css の色・角丸は、部品のトークン(--ui-〜)として、同じ値で持つ", () => {
    const names = Object.keys(dv).filter((n) => !n.startsWith("logo-"));
    expect(names.length).toBeGreaterThanOrEqual(11);
    for (const n of names) expect(own[`ui-${n}`], `--ui-${n}`).toBe(dv[n]);
  });

  it("ロゴの 2 色は、mock.css と同じ名前・同じ値である", () => {
    for (const n of ["logo-on-light", "logo-on-dark"]) expect(own[n], `--${n}`).toBe(dv[n]);
  });
});

// WCAG の相対輝度とコントラスト比。
const channel = (v: number) => (v / 255 <= 0.03928 ? v / 255 / 12.92 : ((v / 255 + 0.055) / 1.055) ** 2.4);
const luminance = (hex: string) => {
  const [r, g, b] = [1, 3, 5].map((i) => channel(parseInt(hex.slice(i, i + 2), 16)));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const contrast = (a: string, b: string) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
};

describe("部品の色の組み合わせは、文字が読める明るさの差を保つ", () => {
  const c = (name: string) => own[`ui-${name}`];

  it("文字と面の組み合わせは、通常の文字の基準(4.5:1)を満たす", () => {
    const pairs: [string, string][] = [
      ["text", "bg"],
      ["text-muted", "bg"],
      ["text-muted", "surface-2"],
      ["on-accent", "accent"],
      ["danger", "bg"],
      ["danger", "surface-2"],
      ["text", "danger-surface"],
      ["danger", "danger-surface"],
      ["bg", "danger"],
      ["bg", "text"],
    ];
    for (const [fg, bg] of pairs) expect(contrast(c(fg), c(bg)), `${fg} on ${bg}`).toBeGreaterThanOrEqual(4.5);
  });

  it("入力欄の枠は、白い面に対して、部品の境界の基準(3:1)を満たす", () => {
    expect(contrast(c("control-border"), c("bg"))).toBeGreaterThanOrEqual(3);
  });

  it("黄(--ui-accent)は白い面の上の文字・線に使えない明るさなので、面の塗りだけに使う(白に対して 3:1 未満)", () => {
    expect(contrast(c("accent"), c("bg"))).toBeLessThan(3);
  });
});
