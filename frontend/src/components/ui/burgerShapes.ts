import parts from './burgerParts.json'

// 評価のバーガー(線画を、評価の分だけ下から水位のように塗る)の図形の組み立て。元は design/redesign/rating-icons.js の
// resolve(案 B)。形・色・大きさ・線の太さは burgerParts.json(デザインの PARTS・SIMPLE・SIZE の写し)にあり、
// burgerShapes.test.ts が、デザインの実行結果との一致を確かめる。座標などの数字を .ts に書かない。
export type BurgerSize = 'lg' | 'md' | 'sm' | 'xs'

interface Detail {
  d: string
  at: number[]
  fill?: string
  stroke?: string
  w?: number
}
interface Part {
  id: string
  unit: number
  color: string
  d: string
  details: Detail[]
}

export interface BurgerShape {
  part: string
  d: string
  // null は塗らない。t があるときは、下から t の割合まで color で、残りは白。
  fill: { color: string; t?: number } | null
  stroke: string
  sw: number
}
export interface BurgerIcon {
  vb: [number, number]
  width: number
  height: number
  shapes: BurgerShape[]
}

export const WHITE = '#ffffff'

// デザインの丸め(小数 2 桁)と同じ。
const round2 = (v: number) => Math.round(v * 1e2) / 1e2
const CURVE_STEPS = 40

// パス(M・L・C・Z だけ)の外接の四角 [x, y, 幅, 高さ]。曲線は刻んで測る。
function bboxOf(d: string): [number, number, number, number] {
  const tokens = d.match(/[MLCZ]|-?[\d.]+/g) ?? []
  let i = 0
  let cx = 0
  let cy = 0
  let x0 = Infinity
  let y0 = Infinity
  let x1 = -Infinity
  let y1 = -Infinity
  const add = (x: number, y: number) => {
    x0 = Math.min(x0, x)
    y0 = Math.min(y0, y)
    x1 = Math.max(x1, x)
    y1 = Math.max(y1, y)
  }
  const num = () => parseFloat(tokens[i++])
  while (i < tokens.length) {
    const c = tokens[i++]
    if (c === 'M' || c === 'L') {
      cx = num()
      cy = num()
      add(cx, cy)
    } else if (c === 'C') {
      const a = [cx, cy, num(), num(), num(), num(), num(), num()]
      for (let s = 1; s <= CURVE_STEPS; s++) {
        const u = s / CURVE_STEPS
        const v = 1 - u
        add(
          v ** 3 * a[0] + 3 * v * v * u * a[2] + 3 * v * u * u * a[4] + u ** 3 * a[6],
          v ** 3 * a[1] + 3 * v * v * u * a[3] + 3 * v * u * u * a[5] + u ** 3 * a[7],
        )
      }
      cx = a[6]
      cy = a[7]
    }
  }
  return [round2(x0), round2(y0), round2(x1 - x0), round2(y1 - y0)]
}

const bboxCache = new Map<string, [number, number, number, number]>()
const bbox = (d: string) => {
  let b = bboxCache.get(d)
  if (!b) {
    b = bboxOf(d)
    bboxCache.set(d, b)
  }
  return b
}

const FULL_SIZE = parts.size.lg.w

// 評価の割合 ratio(0〜1。範囲外は、0 以下は空・1 以上は満タンと同じ結果になる)と大きさから、描く図形の一覧を作る。
// 小(sm)・極小(xs)は、細部と細かい縁をやめた簡略版の部品を使う(デザインのとおり)。
export function resolveBurger(ratio: number, size: BurgerSize): BurgerIcon {
  const sz = parts.size[size]
  const scale = sz.w / parts.vb[0]
  const sw = round2(sz.stroke / scale)
  const level = parts.bottom - (parts.bottom - parts.top) * ratio
  const list: Part[] = size === 'sm' || size === 'xs' ? parts.simple[size] : parts.parts
  const shapes: BurgerShape[] = []
  for (const p of list) {
    const [, y, , h] = bbox(p.d)
    const full = y >= level
    const empty = !full && y + h <= level
    const fill = empty
      ? { color: WHITE }
      : full
        ? { color: p.color }
        : { color: p.color, t: round2((y + h - level) / h) }
    shapes.push({ part: p.id, d: p.d, fill, stroke: empty ? parts.empty : parts.ink, sw })
    if (!sz.detail || empty) continue
    for (const q of p.details) {
      // 細部は、塗ってある側(水位より下)にあるものだけ描く。
      if (q.at[1] < level) continue
      shapes.push({
        part: `${p.id}の細部`,
        d: q.d,
        fill: q.fill ? { color: q.fill } : null,
        stroke: q.stroke ?? parts.mark,
        sw: round2(Math.max(0.8, ((q.w ?? 0.9) * sz.w) / FULL_SIZE) / scale),
      })
    }
  }
  return { vb: [parts.vb[0], parts.vb[1]], width: sz.w, height: round2((sz.w * parts.vb[1]) / parts.vb[0]), shapes }
}

// 数字に出す値(小数 1 桁)。水位も、この値で決める(数字と水位がずれないように)。
export const roundRating = (v: number) => Math.round(v * 10) / 10
