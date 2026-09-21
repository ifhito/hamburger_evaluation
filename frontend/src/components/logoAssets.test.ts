import { describe, it, expect } from "vitest";

// デザインのアセット(リポジトリの design/assets/logo/)。frontend だけを取り出した環境(compose で frontend だけを
// 取り付けたコンテナなど)では、design/ が無いので、その場合は、このファイルの一致の確認を飛ばす(CI では全体が
// 取り出されるので、実行される)。
const designSvgs = import.meta.glob<string>("../../../design/assets/logo/*.svg", { query: "?raw", import: "default", eager: true });
const designPngs = import.meta.glob<string>("../../../design/assets/logo/*.png", { query: "?raw", import: "default", eager: true });
const hasDesign = Object.keys(designSvgs).length > 0;

const ownSvgs = import.meta.glob<string>("../assets/logo/*.svg", { query: "?raw", import: "default", eager: true });
const publicFiles = import.meta.glob<string>("../../public/*", { query: "?raw", import: "default", eager: true });
const indexHtml = import.meta.glob<string>("../../index.html", { query: "?raw", import: "default", eager: true });

const byName = (files: Record<string, string>) => Object.fromEntries(Object.entries(files).map(([p, v]) => [p.split("/").pop()!, v]));
const design = { ...byName(designSvgs), ...byName(designPngs) };
const publicByName = byName(publicFiles);

describe.skipIf(!hasDesign)("デザインのアセットとの一致", () => {
  it("frontend の記号の SVG は、デザインのアセットと同じ内容である", () => {
    const own = byName(ownSvgs);
    expect(Object.keys(own)).toEqual(["burgerstack-mark.svg"]);
    for (const [name, svg] of Object.entries(own)) expect(svg, name).toBe(design[name]);
  });

  it("デザインのアセットに、波を省いた小さい版は無い(どの大きさでも、波つきの記号を使う)", () => {
    expect(Object.keys(design).filter((name) => /small/.test(name))).toEqual([]);
  });

  it("public/ のファビコンとアイコンは、デザインのアセットと同じ内容である", () => {
    const names = ["favicon.svg", "favicon-32.png", "favicon-48.png", "apple-touch-icon.png"];
    expect(Object.keys(publicByName).sort()).toEqual([...names].sort());
    for (const name of names) expect(publicByName[name], name).toBe(design[name]);
  });
});

describe("ファビコンの配線", () => {
  it("index.html は、SVG・32px・48px のファビコンと apple-touch-icon を参照する", () => {
    const html = Object.values(indexHtml)[0];
    expect(html).toContain('<link rel="icon" type="image/svg+xml" href="/favicon.svg"');
    expect(html).toContain('<link rel="icon" type="image/png" sizes="32x32" href="/favicon-32.png"');
    expect(html).toContain('<link rel="icon" type="image/png" sizes="48x48" href="/favicon-48.png"');
    expect(html).toContain('<link rel="apple-touch-icon" href="/apple-touch-icon.png"');
  });

  it("SVG のファビコンは、記号と同じ path(波つき)と線の太さを使う", () => {
    const mark = Object.values(ownSvgs)[0];
    const favicon = publicByName["favicon.svg"];
    expect(/<path d="([^"]+)"/.exec(favicon)?.[1]).toBe(/<path d="([^"]+)"/.exec(mark)?.[1]);
    expect(/stroke-width="([^"]+)"/.exec(favicon)?.[1]).toBe(/stroke-width="([^"]+)"/.exec(mark)?.[1]);
  });

  it("SVG のファビコンは、暗いタブでは今の黄、明るいタブでは深い黄に切り替わる", () => {
    const svg = publicByName["favicon.svg"];
    expect(svg).toMatch(/path\{stroke:#b88f13\}@media \(prefers-color-scheme:dark\)\{path\{stroke:#e9b824\}\}/);
  });

  it("192px と 512px のアイコンは、マニフェストがないので、置いていない", () => {
    expect(Object.keys(publicByName).some((n) => /icon-(192|512)/.test(n))).toBe(false);
  });
});
