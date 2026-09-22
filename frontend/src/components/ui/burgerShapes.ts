import parts from './burgerParts.json'

// 評価のバーガー(1 食材 = 1 本の波打つ線。水位より下は食材の色、上は消灯のグレー)の図形の組み立て。
// 元は design/redesign/rating-icons.js の resolve(案 B)。形・色・大きさ・線の太さは burgerParts.json
// (デザインの PARTS・SIZE の写し)にあり、burgerShapes.test.ts が、デザインの実行結果との一致を確かめる。
// 座標などの数字を .ts に書かない。
export type BurgerSize = 'lg' | 'md' | 'sm' | 'xs'

interface Part {
  id: string
  unit: number
  y: number
  color: string
  d: string
}

export interface BurgerShape {
  part: string
  d: string
  stroke: string
  sw: number
}
export interface BurgerIcon {
  vb: [number, number]
  width: number
  height: number
  shapes: BurgerShape[]
}

// デザインの丸め(小数 2 桁)と同じ。
const round2 = (v: number) => Math.round(v * 1e2) / 1e2

// 極小(24px)だけの簡略版: 6 本の線は隙間が潰れて見分けられないため、3 本(下のバンズ・チーズ(具の代表)・上のバンズ)に
// 間引く(design/redesign/rating-icons.js と同じ)。同じパス・色をそのまま使い、線を新しく作らない。
const XS_IDS = ['下のバンズ', 'チーズ', '上のバンズ']

// 評価の割合 ratio(0〜1。範囲外は、0 以下は空・1 以上は満タンと同じ結果になる)と大きさから、描く線の一覧を作る。
export function resolveBurger(ratio: number, size: BurgerSize): BurgerIcon {
  const sz = parts.size[size]
  const scale = sz.w / parts.vb[0]
  const sw = round2(sz.stroke / scale)
  const level = parts.bottom - (parts.bottom - parts.top) * Math.max(0, Math.min(ratio, 1))
  const list = (parts.parts as Part[]).filter((p) => size !== 'xs' || XS_IDS.includes(p.id))
  const shapes: BurgerShape[] = list.map((p) => ({
    part: p.id,
    d: p.d,
    stroke: p.y >= level ? p.color : parts.empty,
    sw,
  }))
  return { vb: [parts.vb[0], parts.vb[1]], width: sz.w, height: round2((sz.w * parts.vb[1]) / parts.vb[0]), shapes }
}

// 数字に出す値(小数 1 桁)。水位も、この値で決める(数字と水位がずれないように)。
export const roundRating = (v: number) => Math.round(v * 10) / 10
