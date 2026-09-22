// 評価のバーガーのアイコン(見本の HTML 用)。線画のバーガーを、評価の分だけ下から塗る。
//   案 B(採用): 水位。1 つのバーガー全体を、下から「評価 ÷ 最大値」の高さまで塗る(連続値)。
//   案 A(参考): 部品ごと。下から 1 点 = 1 部品(バンズ・パティ・チーズ・野菜・上のバンズ)。半分は、その部品の左半分。
// 図形は SVG のパス(M・L・C・Z の絶対座標だけ)で、capture_mock.js が、この一覧(shapes)を、そのまま Penpot のパスにする。
// 見た目の値(色・大きさ・線の太さ)は、ここが 1 か所の元。数字は、必ずアイコンのそばに書く(形や色だけで伝えない)。
(() => {
  const VB = { w: 120, h: 100 };
  const TOP = 2, BOTTOM = 98; // バーガー全体の上端と下端(水位の 0% と 100%)
  const INK = '#111111', EMPTY = '#8e8e89';
  const COLOR = { bun: '#ff7a00', patty: '#ff3ea5', cheese: '#f9f002', lettuce: '#39ff14', tomato: '#b026ff', seed: '#f7edd3', pocket: '#f2b6a8', mark: '#a2795f', vein: '#c5dda8' };
  // 大きさ(アイコンの幅 px)と、線の太さ(px)。小さいほど、線を細くしすぎない。細部(ごま・焼き目など)は、大きい 2 つだけ
  const SIZE = { lg: { w: 160, stroke: 3, detail: true }, md: { w: 64, stroke: 2, detail: true }, sm: { w: 40, stroke: 1.5, detail: false }, xs: { w: 24, stroke: 1.25, detail: false } };
  const n2 = (v) => Math.round(v * 100) / 100;

  // ---- パスの部品(M・L・C・Z だけを使う) ----
  const K = 0.5523;
  const rrect = (x, y, w, h, r) => { const k = r * K; return `M ${x + r} ${y} L ${x + w - r} ${y} C ${x + w - r + k} ${y} ${x + w} ${y + r - k} ${x + w} ${y + r} L ${x + w} ${y + h - r} C ${x + w} ${y + h - r + k} ${x + w - r + k} ${y + h} ${x + w - r} ${y + h} L ${x + r} ${y + h} C ${x + r - k} ${y + h} ${x} ${y + h - r + k} ${x} ${y + h - r} L ${x} ${y + r} C ${x} ${y + r - k} ${x + r - k} ${y} ${x + r} ${y} Z`; };
  const scallops = (x0, x1, y, count, depth) => { // 左から右へ、下に膨らむ縁(ちぢれたレタス)
    const dx = (x1 - x0) / count, c = depth / 0.75;
    let d = '';
    for (let i = 0; i < count; i++) { const x = x0 + dx * i; d += ` C ${n2(x + dx * 0.15)} ${n2(y + c)} ${n2(x + dx * 0.85)} ${n2(y + c)} ${n2(x + dx)} ${y}`; }
    return d;
  };

  // 下から重ねる順(上に重なる部品が後)。unit は案 A で「何点目で塗るか」(1〜5)
  const PARTS = [
    { id: '下のバンズ', unit: 1, color: COLOR.bun, d: 'M 8 82 L 112 82 C 112 91 108 98 98 98 L 22 98 C 12 98 8 91 8 82 Z', details: [] },
    { id: 'パティ', unit: 2, color: COLOR.patty, d: 'M 5 71.5 C 5 64 9 62 17 62 L 103 62 C 111 62 115 64 115 71.5 C 115 79 111 81 103 81 L 17 81 C 9 81 5 79 5 71.5 Z', details: [] },
    { id: 'チーズ', unit: 3, color: COLOR.cheese, d: 'M 8 54 L 112 54 L 112 61 L 98 61 C 98 81 88 81 88 61 L 60 61 C 60 69 54 69 54 61 L 32 61 C 32 75.7 22 75.7 22 61 L 8 61 Z', details: [] },
    { id: 'トマト', unit: 4, color: COLOR.tomato, d: [rrect(10, 46, 32, 10, 5), rrect(44, 46, 32, 10, 5), rrect(78, 46, 32, 10, 5)].join(' '), details: [] },
    { id: 'レタス', unit: 4, color: COLOR.lettuce, d: `M 2 38 L 2 44${scallops(2, 118, 44, 9, 5.25)} L 118 38 Z`, details: [] },
    { id: '上のバンズ', unit: 5, color: COLOR.bun, d: 'M 6 36 C 6 15 30 2 60 2 C 90 2 114 15 114 36 C 114 38 113 39 111 39 L 9 39 C 7 39 6 38 6 36 Z', details: [] },
  ];

  // 小さいサイズ(案 B だけ)の簡略版: 細部と細かい縁(ちぢれ・トマトの輪切り・しずく)をやめて、部品を太くし、水位の高さが読めるようにする。
  // 40px は 5 部品(バンズ・パティ・チーズ・レタス・上のバンズ)、24px は 3 部品(上のバンズ・パティ(具)・下のバンズ)。
  const BUN_TOP = 'M 6 36 C 6 15 30 2 60 2 C 90 2 114 15 114 36 C 114 38 113 39 111 39 L 9 39 C 7 39 6 38 6 36 Z';
  const BUN_BOTTOM = 'M 8 82 L 112 82 C 112 91 108 98 98 98 L 22 98 C 12 98 8 91 8 82 Z';
  const SIMPLE = {
    sm: [
      { id: '下のバンズ', unit: 1, color: COLOR.bun, d: BUN_BOTTOM, details: [] },
      { id: 'パティ', unit: 2, color: COLOR.patty, d: 'M 5 70 C 5 63 9 61 17 61 L 103 61 C 111 61 115 63 115 70 C 115 78 111 82 103 82 L 17 82 C 9 82 5 78 5 70 Z', details: [] },
      { id: 'チーズ', unit: 3, color: COLOR.cheese, d: rrect(6, 50, 108, 12, 3), details: [] },
      { id: 'レタス', unit: 4, color: COLOR.lettuce, d: rrect(2, 37, 116, 14, 7), details: [] },
      { id: '上のバンズ', unit: 5, color: COLOR.bun, d: BUN_TOP, details: [] },
    ],
    xs: [
      { id: '下のバンズ', unit: 1, color: COLOR.bun, d: BUN_BOTTOM, details: [] },
      { id: 'パティ(具)', unit: 3, color: COLOR.patty, d: rrect(4, 40, 112, 42, 12), details: [] },
      { id: '上のバンズ', unit: 5, color: COLOR.bun, d: BUN_TOP, details: [] },
    ],
  };

  // パスの外接の四角(曲線を刻んで測る)。Penpot と SVG のグラデーションの位置合わせに使う
  const bboxOf = (d) => {
    const t = d.match(/[MLCZ]|-?[\d.]+/g);
    let i = 0, cx = 0, cy = 0, x0 = 1e9, y0 = 1e9, x1 = -1e9, y1 = -1e9;
    const add = (x, y) => { x0 = Math.min(x0, x); y0 = Math.min(y0, y); x1 = Math.max(x1, x); y1 = Math.max(y1, y); };
    const num = () => parseFloat(t[i++]);
    while (i < t.length) {
      const c = t[i++];
      if (c === 'M' || c === 'L') { cx = num(); cy = num(); add(cx, cy); }
      else if (c === 'C') {
        const a = [cx, cy, num(), num(), num(), num(), num(), num()];
        for (let s = 1; s <= 40; s++) { const u = s / 40, v = 1 - u; add(v ** 3 * a[0] + 3 * v * v * u * a[2] + 3 * v * u * u * a[4] + u ** 3 * a[6], v ** 3 * a[1] + 3 * v * v * u * a[3] + 3 * v * u * u * a[5] + u ** 3 * a[7]); }
        cx = a[6]; cy = a[7];
      }
    }
    return [n2(x0), n2(y0), n2(x1 - x0), n2(y1 - y0)];
  };
  [...PARTS, ...SIMPLE.sm, ...SIMPLE.xs].forEach((p) => { p.bbox = bboxOf(p.d); p.details.forEach((q) => { q.bbox = bboxOf(q.d); }); });

  // 白黒にしたときの色(CSS の grayscale と同じ係数)
  const grayOf = (hex) => { const v = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16)); const g = Math.round(0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2]); return '#' + g.toString(16).padStart(2, '0').repeat(3); };

  // 評価と大きさから、描く図形の一覧を作る。fill は { color }(全部塗る)/ { color, axis, t }(軸に沿って、始め(y は下、x は左)から t の割合だけ color、残りは白)/ { color: '#ffffff' }(白抜き)
  // variant: 'B' 水位 / 'A' 部品ごと。value は、数字に出す値(1 桁に丸めた値)。max は評価の最大値
  const resolve = ({ variant = 'B', value = 0, max = 5, size = 'md', gray = false }) => {
    const sz = SIZE[size], scale = sz.w / VB.w, sw = n2(sz.stroke / scale);
    const tone = (c) => (gray ? grayOf(c) : c);
    const shapes = [];
    const level = BOTTOM - (BOTTOM - TOP) * (Math.max(0, Math.min(value, max)) / max); // 水位の y(これより下が塗られる)
    const unitValue = (value / max) * 5; // 案 A は 5 部品。最大値が違っても 5 段の割合にする
    const stateOf = (p) => {
      if (variant === 'A') { if (unitValue >= p.unit) return { full: true }; if (unitValue >= p.unit - 0.5) return { half: true }; return { empty: true }; }
      const [, y, , h] = p.bbox;
      if (y >= level) return { full: true };
      if (y + h <= level) return { empty: true };
      return { part: true, t: n2((y + h - level) / h) };
    };
    (variant === 'B' && SIMPLE[size] ? SIMPLE[size] : PARTS).forEach((p) => {
      const st = stateOf(p);
      const [bx, , bw] = p.bbox;
      const fill = st.empty ? { color: '#ffffff' } : st.full ? { color: tone(p.color) } : st.half ? { color: tone(p.color), axis: 'x', t: n2((60 - bx) / bw) } : { color: tone(p.color), axis: 'y', t: st.t };
      shapes.push({ part: p.id, d: p.d, fill, stroke: st.empty ? EMPTY : INK, sw, bbox: p.bbox });
      if (!sz.detail || st.empty) return;
      p.details.forEach((q) => { // 細部は、塗ってある側にあるものだけ描く
        const [ax, ay] = q.at;
        if (variant === 'B' ? ay < level : st.half && ax > 60) return;
        shapes.push({ part: p.id + 'の細部', d: q.d, fill: q.fill ? { color: tone(q.fill) } : null, stroke: tone(q.stroke || COLOR.mark), sw: n2(Math.max(0.8, (q.w || 0.9) * sz.w / 160) / scale), bbox: q.bbox });
      });
    });
    return { vb: [VB.w, VB.h], width: sz.w, height: n2(sz.w * VB.h / VB.w), shapes };
  };

  let uid = 0;
  const svgOf = (icon) => {
    let defs = '', body = '';
    icon.shapes.forEach((s) => {
      let fill = s.fill ? s.fill.color : 'none';
      if (s.fill && s.fill.axis) {
        const id = `bg${++uid}`, [x1, y1, x2, y2] = s.fill.axis === 'y' ? [0, 1, 0, 0] : [0, 0, 1, 0], t = s.fill.t;
        defs += `<linearGradient id="${id}" x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}"><stop offset="0" stop-color="${s.fill.color}"/><stop offset="${t}" stop-color="${s.fill.color}"/><stop offset="${n2(t + 0.0001)}" stop-color="#ffffff"/><stop offset="1" stop-color="#ffffff"/></linearGradient>`;
        fill = `url(#${id})`;
      }
      body += `<path d="${s.d}" fill="${fill}" stroke="${s.stroke}" stroke-width="${s.sw}" stroke-linejoin="round" stroke-linecap="round"/>`;
    });
    return `<svg viewBox="0 0 ${icon.vb[0]} ${icon.vb[1]}" width="${icon.width}" height="${icon.height}" xmlns="http://www.w3.org/2000/svg"><defs>${defs}</defs>${body}</svg>`;
  };

  // .burger の要素(data-score・data-size・data-variant・data-gray・data-max)に、アイコンを描く。data-decor は飾り(読み上げない)
  const mount = (el, lang) => {
    const max = parseFloat(el.dataset.max || '5');
    const value = Math.round(parseFloat(el.dataset.score || '0') * 10) / 10; // 数字に出す 1 桁の値と同じにする
    const icon = resolve({ variant: el.dataset.variant || 'B', value, max, size: el.dataset.size || 'md', gray: el.hasAttribute('data-gray') });
    el.innerHTML = svgOf(icon);
    el.firstElementChild.__icon = { ...icon, name: `バーガーの評価 ${value}` };
    if (el.hasAttribute('data-decor')) el.setAttribute('aria-hidden', 'true');
    else { el.setAttribute('role', 'img'); el.setAttribute('aria-label', lang === 'en' ? `${value} out of ${max}` : `${max} 段階中 ${value}`); }
  };

  window.Burger = { VB, SIZE, COLOR, INK, EMPTY, PARTS, SIMPLE, resolve, svgOf, mount, grayOf };
})();
