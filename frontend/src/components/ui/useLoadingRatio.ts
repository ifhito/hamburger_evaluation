import { useEffect, useState } from 'react'

// 読み込み中のバーガー(見た目だけの水位。評価ではない)が、下から繰り返し満ちていく水位。6 食材ぶんの段(0〜1 を
// 6 等分)を、一定の間隔でめぐる。prefers-reduced-motion のときは、動かさず、1 段目で止める。
const LOADING_STEPS = [0, 1 / 6, 2 / 6, 3 / 6, 4 / 6, 5 / 6, 1]
const LOADING_STEP_MS = 220

export function useLoadingRatio() {
  const [i, setI] = useState(0)
  useEffect(() => {
    // jsdom (テスト環境) は matchMedia を持たない。持たない環境は、動かして構わない扱いにする。
    if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const id = setInterval(() => setI((n) => (n + 1) % LOADING_STEPS.length), LOADING_STEP_MS)
    return () => clearInterval(id)
  }, [])
  return LOADING_STEPS[i]
}
