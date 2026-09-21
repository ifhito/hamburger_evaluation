import { describe, it, expect } from "vitest";
import en from "../locale/en";

// frontend のソース(テストとストーリーを除く)を、文字列として読む。
const sources = import.meta.glob<string>("/src/**/*.{ts,tsx}", { query: "?raw", import: "default", eager: true });
const productionSources = Object.entries(sources).filter(([path]) => !/\.(test|stories)\.tsx?$/.test(path));

describe("backend の規則(文字数の上限・パスワードの長さ)を frontend に書かない", () => {
  // 上限は、GET /meta が返す値を使う。次の数字は backend の現在の上限で(コメント 2000・自己紹介と却下メモ 500・
  // バーガー名とショップ名 100・ユーザー名 50・パスワードの最大 72)、ソースに直書きされたら失敗にする。
  // 上限を変えても、GET /meta から取る限りこのテストは直さなくてよい。直書きの検出が目的なので、
  // 新しく別の上限を足したときは、その数字もここに足す。
  const backendLimits = /\b(2000|500|100|50|72)\b/;

  it("ソースに、backend の上限の数字が直書きされていない", () => {
    expect(productionSources.length).toBeGreaterThan(20);
    const found = productionSources.filter(([, code]) => backendLimits.test(code)).map(([path]) => path);
    expect(found).toEqual([]);
  });

  it("入力欄に maxLength を直書きしていない(上限は GET /meta の値で見せる)", () => {
    const found = productionSources.filter(([, code]) => /maxLength/.test(code)).map(([path]) => path);
    expect(found).toEqual([]);
  });

  it("パスワードの説明文は、最小・最大を差し込む形で、数字を持たない", () => {
    expect(en.auth.passwordHint).toContain("{{min}}");
    expect(en.auth.passwordHint).toContain("{{max}}");
    expect(en.auth.passwordHint).not.toMatch(/\b(8|72)\b/);
  });
});
