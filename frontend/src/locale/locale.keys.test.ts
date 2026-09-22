import { describe, expect, it } from "vitest";
import en from "./en";
import ja from "./ja";

// キーの過不足を検出する(AC3)。値ではなく「ネストしたキーのパスの集合」を比べるので、
// 文言の翻訳そのものは対象にしない。
function keyPaths(obj: unknown, prefix = ""): string[] {
  if (typeof obj !== "object" || obj === null) return [prefix];
  return Object.entries(obj as Record<string, unknown>).flatMap(([key, value]) => keyPaths(value, prefix ? `${prefix}.${key}` : key));
}

describe("ja.ts と en.ts のキー", () => {
  const enKeys = keyPaths(en).sort();
  const jaKeys = keyPaths(ja).sort();

  it("過不足がない(同じキーの集合を持つ)", () => {
    expect(jaKeys).toEqual(enKeys);
  });

  it("値はすべて空でない文字列である", () => {
    for (const [name, dict] of [
      ["en", en],
      ["ja", ja],
    ] as const) {
      for (const path of keyPaths(dict)) {
        const value = path.split(".").reduce<unknown>((acc, key) => (acc as Record<string, unknown>)[key], dict);
        expect(typeof value, `${name}.${path}`).toBe("string");
        expect((value as string).length, `${name}.${path}`).toBeGreaterThan(0);
      }
    }
  });
});
