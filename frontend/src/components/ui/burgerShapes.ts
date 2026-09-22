import parts from './burgerParts.json'

// 評価のバーガー(1 食材 = 1 本の波打つ線。点灯は食材の色の実線、消灯は薄いグレーの点線)の図形の組み立て。
// 元は design/redesign/rating-icons.js の resolve()。形・色・大きさ・線の太さは burgerParts.json
// (デザインの PARTS・SIZE の写し)にあり、burgerShapes.test.ts が、デザインの実行結果との一致を確かめる。
// 座標などの数字を .ts に書かない。
export type BurgerSize = 'lg' | 'md' | 'sm' | 'xs'

// 案B(水位。6 本を、下から何番目かで均等な間隔に割り、評価 ÷ 最大値がその割合を超えたら点灯): 小数の違いが
// 見た目に出て、評価の最大値でしか満タンにならない。平均評価(小数になる)に使う。
// 案A(部品ごとの段階。下から何点目で灯るか、より下が点灯): 段階がはっきり分かる。1 件の評価・評価の入力
// (必ず整数)に使う(小数は区別できない)。
export type BurgerVariant = 'level' | 'stepped'

interface Part {
  id: string
  unit: number
  rank: number
  color: string
  d: string
}

export interface BurgerShape {
  part: string
  d: string
  stroke: string
  sw: number
  // 点線の間隔(dash gap)。null は実線(点灯)。
  dash: string | null
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
export function resolveBurger(ratio: number, size: BurgerSize, variant: BurgerVariant = 'level'): BurgerIcon {
  const sz = parts.size[size]
  const scale = sz.w / parts.vb[0]
  const sw = round2(sz.stroke / scale)
  const r = Math.max(0, Math.min(ratio, 1))
  const unitValue = r * 5
  const allParts = parts.parts as Part[]
  const list = allParts.filter((p) => size !== 'xs' || XS_IDS.includes(p.id))
  const shapes: BurgerShape[] = list.map((p) => {
    const lit = variant === 'stepped' ? unitValue >= p.unit : r >= p.rank / allParts.length
    return { part: p.id, d: p.d, stroke: lit ? p.color : parts.empty, sw, dash: lit ? null : `${round2(sw * 1.6)} ${round2(sw * 1.6)}` }
  })
  return { vb: [parts.vb[0], parts.vb[1]], width: sz.w, height: round2((sz.w * parts.vb[1]) / parts.vb[0]), shapes }
}

// 数字に出す値(小数 1 桁)。水位・段階も、この値で決める(数字とずれないように)。
export const roundRating = (v: number) => Math.round(v * 10) / 10
