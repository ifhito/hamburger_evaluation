import { describe, it, expect } from "vitest";
import parts from "./burgerParts.json";
import { resolveBurger, roundRating, type BurgerSize } from "./burgerShapes";

// デザインの見本(リポジトリの design/redesign/rating-icons.js)。frontend だけを取り出した環境では design/ が無いので、
// その場合は、デザインとの一致の確認を飛ばす(CI では全体が取り出されるので、実行される)。
const designJs = import.meta.glob<string>("../../../../design/redesign/rating-icons.js", { query: "?raw", import: "default", eager: true });
const source = Object.values(designJs)[0];

// CI では、デザインのファイルが無い(移動・改名された)ときは、飛ばさず失敗にする(飛ばすのは、frontend だけを取り出した環境のため)。
const inCi = Boolean((globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env.CI);
it.runIf(inCi)("CI では、デザインの見本のファイルが見つかる", () => expect(source).toBeTruthy());

type DesignShape = { part: string; d: string; fill: null; stroke: string; sw: number };
type Design = {
  SIZE: typeof parts.size;
  PARTS: { id: string; unit: number; y: number; color: string; d: string }[];
  resolve: (o: { value: number; max: number; size: BurgerSize }) => { width: number; height: number; shapes: DesignShape[] };
};
const load = (): Design => {
  const win: { Burger?: Design } = {};
  new Function("window", source)(win);
  return win.Burger!;
};

const sizes: BurgerSize[] = ["lg", "md", "sm", "xs"];

describe.skipIf(!source)("水位のバーガーは、デザインの見本と同じ絵になる", () => {
  const design = source ? load() : ({} as Design);

  it("図形の元データ(部品・大きさ・線の太さ)は、デザインの見本と同じである", () => {
    expect(parts.size).toEqual(design.SIZE);
    const strip = (list: Design["PARTS"]) => list.map(({ id, unit, y, color, d }) => ({ id, unit, y, color, d }));
    expect(strip(parts.parts as Design["PARTS"])).toEqual(strip(design.PARTS));
  });

  it("評価 0〜最大値を 0.1 刻みで変え、どの大きさでも、図形・点灯の色・線の太さが、デザインの見本と一致する", () => {
    for (const max of [5, 3, 10]) {
      for (let tenths = 0; tenths <= max * 10; tenths++) {
        const value = tenths / 10;
        for (const size of sizes) {
          const want = design.resolve({ value, max, size });
          const got = resolveBurger(value / max, size);
          const label = `${size} ${value}/${max}`;
          expect(got.width, label).toBe(want.width);
          expect(got.height, label).toBe(want.height);
          expect(got.shapes.length, label).toBe(want.shapes.length);
          got.shapes.forEach((s, i) => {
            const w = want.shapes[i];
            expect(s.part, `${label} ${i}`).toBe(w.part);
            expect(s.d, `${label} ${w.part}`).toBe(w.d);
            expect(s.stroke, `${label} ${w.part} 線の色`).toBe(w.stroke);
            expect(s.sw, `${label} ${w.part} 線の太さ`).toBe(w.sw);
          });
        }
      }
    }
  });

  it("範囲外の評価(負・最大値超え)は、空・満タンに丸められ、例外にならない", () => {
    for (const size of sizes) {
      expect(resolveBurger(-1, size)).toEqual(resolveBurger(0, size));
      expect(resolveBurger(3, size)).toEqual(resolveBurger(1, size));
    }
  });
});

describe("水位での点灯・消灯", () => {
  it("評価が 0 のときは、すべての食材が消灯の色で、点灯した食材がない", () => {
    const icon = resolveBurger(0, "lg");
    expect(icon.shapes.every((s) => s.stroke === parts.empty)).toBe(true);
  });

  it("評価が最大のときは、すべての食材が自分の色で点灯し、消灯した食材がない", () => {
    const icon = resolveBurger(1, "lg");
    expect(icon.shapes.every((s) => s.stroke !== parts.empty)).toBe(true);
  });

  it("評価が上がるほど、点灯している食材が増える(減ることはない)", () => {
    const lit = (ratio: number) => resolveBurger(ratio, "lg").shapes.filter((s) => s.stroke !== parts.empty).length;
    const counts = [0, 0.2, 0.4, 0.6, 0.8, 1].map(lit);
    expect(counts).toEqual([...counts].sort((a, b) => a - b));
    expect(counts[0]).toBe(0);
    expect(counts[counts.length - 1]).toBeGreaterThan(counts[1]);
  });

  it("極小(24px)は、下のバンズ・チーズ・上のバンズの 3 本だけに間引かれる", () => {
    const icon = resolveBurger(1, "xs");
    expect(icon.shapes.map((s) => s.part)).toEqual(["下のバンズ", "チーズ", "上のバンズ"]);
    expect(resolveBurger(1, "lg").shapes.length).toBe(6);
  });
});

describe("数字に出す値", () => {
  it("小数 1 桁に丸める", () => {
    expect(roundRating(4.46)).toBe(4.5);
    expect(roundRating(4.44)).toBe(4.4);
    expect(roundRating(4)).toBe(4);
  });
});
