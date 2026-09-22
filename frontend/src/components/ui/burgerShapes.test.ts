import { describe, it, expect } from "vitest";
import parts from "./burgerParts.json";
import { resolveBurger, roundRating, type BurgerSize, type BurgerVariant } from "./burgerShapes";

// デザインの見本(リポジトリの design/redesign/rating-icons.js)。frontend だけを取り出した環境では design/ が無いので、
// その場合は、デザインとの一致の確認を飛ばす(CI では全体が取り出されるので、実行される)。
const designJs = import.meta.glob<string>("../../../../design/redesign/rating-icons.js", { query: "?raw", import: "default", eager: true });
const source = Object.values(designJs)[0];

// CI では、デザインのファイルが無い(移動・改名された)ときは、飛ばさず失敗にする(飛ばすのは、frontend だけを取り出した環境のため)。
const inCi = Boolean((globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env.CI);
it.runIf(inCi)("CI では、デザインの見本のファイルが見つかる", () => expect(source).toBeTruthy());

type DesignShape = { part: string; d: string; fill: null; stroke: string; sw: number; dash: string | null };
type Design = {
  SIZE: typeof parts.size;
  PARTS: { id: string; unit: number; y: number; color: string; d: string }[];
  resolve: (o: { variant?: "A" | "B"; value: number; max: number; size: BurgerSize }) => { width: number; height: number; shapes: DesignShape[] };
};
const load = (): Design => {
  const win: { Burger?: Design } = {};
  new Function("window", source)(win);
  return win.Burger!;
};

const sizes: BurgerSize[] = ["lg", "md", "sm", "xs"];
// frontend の variant名 ↔ デザインの見本の variant名(design/redesign/rating-icons.js の resolve() の引数)。
const DESIGN_VARIANT: Record<BurgerVariant, "A" | "B"> = { level: "B", stepped: "A" };

describe.skipIf(!source)("評価のバーガーは、デザインの見本と同じ絵になる", () => {
  const design = source ? load() : ({} as Design);

  it("図形の元データ(部品・大きさ・線の太さ)は、デザインの見本と同じである", () => {
    expect(parts.size).toEqual(design.SIZE);
    const strip = (list: Design["PARTS"]) => list.map(({ id, unit, y, color, d }) => ({ id, unit, y, color, d }));
    expect(strip(parts.parts as Design["PARTS"])).toEqual(strip(design.PARTS));
  });

  it("評価 0〜最大値を 0.1 刻みで変え、どの大きさ・案(水位/段階)でも、図形・点灯の色・線の太さ・点線が、デザインの見本と一致する", () => {
    for (const variant of ["level", "stepped"] as const) {
      for (const max of [5, 3, 10]) {
        for (let tenths = 0; tenths <= max * 10; tenths++) {
          const value = tenths / 10;
          for (const size of sizes) {
            const want = design.resolve({ variant: DESIGN_VARIANT[variant], value, max, size });
            const got = resolveBurger(value / max, size, variant);
            const label = `${variant} ${size} ${value}/${max}`;
            expect(got.width, label).toBe(want.width);
            expect(got.height, label).toBe(want.height);
            expect(got.shapes.length, label).toBe(want.shapes.length);
            got.shapes.forEach((s, i) => {
              const w = want.shapes[i];
              expect(s.part, `${label} ${i}`).toBe(w.part);
              expect(s.d, `${label} ${w.part}`).toBe(w.d);
              expect(s.stroke, `${label} ${w.part} 線の色`).toBe(w.stroke);
              expect(s.sw, `${label} ${w.part} 線の太さ`).toBe(w.sw);
              expect(s.dash, `${label} ${w.part} 点線`).toBe(w.dash);
            });
          }
        }
      }
    }
  });

  it("範囲外の評価(負・最大値超え)は、空・満タンに丸められ、例外にならない", () => {
    for (const variant of ["level", "stepped"] as const) {
      for (const size of sizes) {
        expect(resolveBurger(-1, size, variant)).toEqual(resolveBurger(0, size, variant));
        expect(resolveBurger(3, size, variant)).toEqual(resolveBurger(1, size, variant));
      }
    }
  });
});

describe("点灯・消灯", () => {
  it("評価が 0 のときは、すべての食材が消灯(グレー・点線)で、点灯した食材がない", () => {
    for (const variant of ["level", "stepped"] as const) {
      const icon = resolveBurger(0, "lg", variant);
      expect(icon.shapes.every((s) => s.stroke === parts.empty && s.dash !== null)).toBe(true);
    }
  });

  it("評価が最大のときは、すべての食材が自分の色・実線で点灯し、消灯した食材がない", () => {
    for (const variant of ["level", "stepped"] as const) {
      const icon = resolveBurger(1, "lg", variant);
      expect(icon.shapes.every((s) => s.stroke !== parts.empty && s.dash === null)).toBe(true);
    }
  });

  it("評価が上がるほど、点灯している食材が増える(減ることはない)", () => {
    for (const variant of ["level", "stepped"] as const) {
      const lit = (ratio: number) => resolveBurger(ratio, "lg", variant).shapes.filter((s) => s.dash === null).length;
      const counts = [0, 0.2, 0.4, 0.6, 0.8, 1].map(lit);
      expect(counts, variant).toEqual([...counts].sort((a, b) => a - b));
      expect(counts[0], variant).toBe(0);
      expect(counts[counts.length - 1], variant).toBeGreaterThan(counts[1]);
    }
  });

  it("極小(24px)は、下のバンズ・チーズ・上のバンズの 3 本だけに間引かれる", () => {
    const icon = resolveBurger(1, "xs");
    expect(icon.shapes.map((s) => s.part)).toEqual(["下のバンズ", "チーズ", "上のバンズ"]);
    expect(resolveBurger(1, "lg").shapes.length).toBe(6);
  });

  it("'stepped'(案A・部品ごとの段階)は、小数点以下が違っても、同じ整数の範囲なら見た目が変わらない", () => {
    const lit = (value: number) => resolveBurger(value / 5, "lg", "stepped").shapes.filter((s) => s.dash === null).length;
    expect(lit(3)).toBe(lit(3.9));
    expect(lit(3)).toBeLessThan(lit(4));
  });

  it("'level'(案B・水位)は、既定(variant を渡さない)の見た目である", () => {
    expect(resolveBurger(0.5, "lg")).toEqual(resolveBurger(0.5, "lg", "level"));
  });
});

describe("数字に出す値", () => {
  it("小数 1 桁に丸める", () => {
    expect(roundRating(4.46)).toBe(4.5);
    expect(roundRating(4.44)).toBe(4.4);
    expect(roundRating(4)).toBe(4);
  });
});
