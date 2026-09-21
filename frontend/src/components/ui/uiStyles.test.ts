import { describe, it, expect } from "vitest";

// 部品の CSS を、文字として確かめる。Vitest の CSS モジュールは、CSS に無い class にも名前を返すので、CSS の規則を消しても、
// TSX のテストは通ってしまう。ここで、TSX が使う class が CSS にあることと、押せる大きさ・フォーカスの枠・隠したラジオが
// キーボードで使えること(display: none にしない)を確かめる。
const tsx = import.meta.glob<string>("./*.tsx", { query: "?raw", import: "default", eager: true });
const css = import.meta.glob<string>("./*.module.css", { query: "?raw", import: "default", eager: true });

const components = Object.entries(tsx).filter(([path]) => !/\.(test|stories)\.tsx$/.test(path));
const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\:]/g, "\\$&");
const block = (file: string, selector: string) => new RegExp(`${escape(selector)}\\s*\\{([^}]*)\\}`).exec(css[`./${file}`])?.[1];

describe("部品の CSS", () => {
  it("部品ごとに CSS モジュールがあり、TSX が使う class は、その CSS に定義されている", () => {
    expect(components.length).toBeGreaterThanOrEqual(8);
    for (const [path, code] of components) {
      const file = /from '\.\/([\w]+\.module\.css)'/.exec(code)?.[1];
      if (!file) continue;
      const sheet = css[`./${file}`];
      expect(sheet, `${path} の ${file}`).toBeTruthy();
      const names = new Set([...code.matchAll(/styles\.(\w+)/g)].map((m) => m[1]));
      // Button は、種類の名前(variant の型の値)をそのまま class の名前にしている
      for (const m of /variant\?: ([^\n]+)/.exec(code)?.[1].matchAll(/'(\w+)'/g) ?? []) names.add(m[1]);
      expect(names.size, path).toBeGreaterThan(0);
      for (const name of names) expect(sheet, `${path}: .${name}`).toMatch(new RegExp(`\\.${name}(?![\\w-])`));
    }
  });

  it("押せる部品(ボタン・入力欄・数字のボタン)は、押せる大きさの基準(44px 以上)を満たす", () => {
    for (const [file, selector] of [["button.module.css", ".btn"], ["field.module.css", ".input"], ["ratingInput.module.css", ".step"]]) {
      const height = /(?:^|[\s;])(?:min-)?height:\s*(\d+)px/.exec(block(file, selector) ?? "")?.[1];
      expect(Number(height), `${file} ${selector}`).toBeGreaterThanOrEqual(44);
    }
  });

  it("キーボードでフォーカスが当たったときは、黒い 2px の枠が出る", () => {
    for (const [file, selector] of [
      ["button.module.css", ".btn:focus-visible"],
      ["field.module.css", ".input:focus-visible"],
      ["ratingInput.module.css", ".step:has(.radio:focus-visible)"],
    ]) {
      expect(block(file, selector), `${file} ${selector}`).toMatch(/outline:\s*2px solid var\(--ui-text\)/);
    }
  });

  it("数字のボタンの本物のラジオは、透明にするだけで、display: none や visibility: hidden にしない(キーボード操作が壊れる)", () => {
    const radio = block("ratingInput.module.css", ".radio");
    expect(radio).toMatch(/opacity:\s*0/);
    expect(radio).not.toMatch(/display:\s*none|visibility:\s*hidden/);
  });

  it("ハイコントラスト表示でも、選んだ数字を、システムの色で見分けられる", () => {
    expect(css["./ratingInput.module.css"]).toMatch(/@media \(forced-colors: active\)\s*\{\s*\.on\s*\{[^}]*Highlight/);
  });

  it("数字のボタンは、最大値が大きくても、折り返して画面からはみ出さない", () => {
    expect(block("ratingInput.module.css", ".steps")).toMatch(/flex-wrap:\s*wrap/);
  });

  it("文字の書体は、トークン(--ui-font)を通す", () => {
    for (const [file, selector] of [
      ["button.module.css", ".btn"],
      ["badge.module.css", ".badge"],
      ["card.module.css", ".card"],
      ["alert.module.css", ".alert"],
      ["field.module.css", ".field"],
      ["states.module.css", ".panel"],
      ["ratingBurger.module.css", ".rate"],
      ["ratingInput.module.css", ".pick"],
    ]) {
      expect(block(file, selector), `${file} ${selector}`).toMatch(/font-family:\s*var\(--ui-font\)/);
    }
  });
});
