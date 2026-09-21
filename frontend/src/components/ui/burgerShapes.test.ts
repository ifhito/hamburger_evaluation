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

type DesignShape = { part: string; d: string; fill: { color: string; t?: number } | null; stroke: string; sw: number };
type Design = {
  SIZE: typeof parts.size;
  PARTS: { id: string; unit: number; color: string; d: string; details: unknown[] }[];
  SIMPLE: Record<"sm" | "xs", { id: string; unit: number; color: string; d: string; details: unknown[] }[]>;
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

  it("図形の元データ(部品・簡略版・大きさ・線の太さ)は、デザインの見本と同じである", () => {
    expect(parts.size).toEqual(design.SIZE);
    const strip = (list: Design["PARTS"]) => list.map(({ id, unit, color, d }) => ({ id, unit, color, d }));
    expect(strip(parts.parts as Design["PARTS"])).toEqual(strip(design.PARTS));
    expect(strip(parts.simple.sm as Design["PARTS"])).toEqual(strip(design.SIMPLE.sm));
    expect(strip(parts.simple.xs as Design["PARTS"])).toEqual(strip(design.SIMPLE.xs));
  });

  it("評価 0〜最大値を 0.1 刻みで変え、どの大きさでも、図形・塗り・水位・線の太さが、デザインの見本と一致する", () => {
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
            expect(s.fill?.color ?? null, `${label} ${w.part} 色`).toBe(w.fill?.color ?? null);
            expect(s.fill?.t, `${label} ${w.part} 水位`).toBe(w.fill?.t);
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

describe("水位の塗り分け", () => {
  const partial = (ratio: number, size: BurgerSize = "lg") => resolveBurger(ratio, size).shapes.filter((s) => s.fill?.t !== undefined);

  it("評価が 0 のときは、すべての部品が白抜きの薄い線で、水位で塗った部品がない", () => {
    const icon = resolveBurger(0, "lg");
    const main = icon.shapes.filter((s) => !s.part.endsWith("の細部"));
    expect(main.every((s) => s.fill?.color === "#ffffff")).toBe(true);
    expect(main.every((s) => s.stroke === parts.empty)).toBe(true);
    expect(icon.shapes.filter((s) => s.part.endsWith("の細部"))).toEqual([]);
  });

  it("評価が最大のときは、すべての部品が色つきで、水位の途中で塗り分ける部品がない", () => {
    const icon = resolveBurger(1, "lg");
    expect(icon.shapes.every((s) => s.fill?.color !== "#ffffff")).toBe(true);
    expect(partial(1)).toEqual([]);
  });

  it("評価が上がるほど、塗ってある部品が増える(細部を除く)", () => {
    const filled = (ratio: number) => resolveBurger(ratio, "lg").shapes.filter((s) => !s.part.endsWith("の細部") && s.fill?.color !== "#ffffff").length;
    const counts = [0, 0.2, 0.4, 0.6, 0.8, 1].map(filled);
    expect(counts).toEqual([...counts].sort((a, b) => a - b));
    expect(counts[0]).toBe(0);
    expect(counts[counts.length - 1]).toBeGreaterThan(counts[1]);
  });

  it("水位の途中にかかる部品は、下から塗る割合(0〜1)を持つ", () => {
    const shapes = partial(0.55);
    expect(shapes.length).toBeGreaterThan(0);
    for (const s of shapes) {
      expect(s.fill!.t!).toBeGreaterThan(0);
      expect(s.fill!.t!).toBeLessThan(1);
    }
  });

  it("小・極小の簡略版は、細部を描かない", () => {
    for (const size of ["sm", "xs"] as const) {
      expect(resolveBurger(1, size).shapes.filter((s) => s.part.endsWith("の細部"))).toEqual([]);
    }
    expect(resolveBurger(1, "lg").shapes.some((s) => s.part.endsWith("の細部"))).toBe(true);
  });
});

describe("数字に出す値", () => {
  it("小数 1 桁に丸める", () => {
    expect(roundRating(4.46)).toBe(4.5);
    expect(roundRating(4.44)).toBe(4.4);
    expect(roundRating(4)).toBe(4);
  });
});
