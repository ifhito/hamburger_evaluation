// 評価のバーガーのアイコン(見本の HTML 用)。1 食材 = 1 本の、波打つ線(管のような線)。塗りつぶしの閉じた形はやめた。
//   線そのものが色を持ち、黒い輪郭線は無い。食材どうしの間には、はっきりした隙間を空ける。
//   水位(評価 ÷ 最大値)より下の食材は、その食材の色の線。水位より上の食材は、灯っていない管のように、薄いグレーの線(塗りは無し・線だけ)。
//   案 A(部品ごと・参考)は、水位の代わりに unit(下から何点目で灯るか)で、同じ点灯/消灯を決める。
// 図形は SVG のパス(M・C の絶対座標だけ)で、capture_mock.js が、この一覧(shapes)を、そのまま Penpot のパスにする。
// 見た目の値(色・大きさ・線の太さ)は、ここが 1 か所の元。数字は、必ずアイコンのそばに書く(形や色だけで伝えない)。
(() => {
  const VB = { w: 120, h: 100 };
  const TOP = 2, BOTTOM = 98; // バーガー全体の上端と下端(水位の 0% と 100%)
  const EMPTY = '#8e8e89'; // 灯っていない(水位より上)の線の色
  const COLOR = { bun: '#d8a45b', patty: '#6b4432', cheese: '#e9b824', lettuce: '#86b25a', tomato: '#d1533d' };
  // 大きさ(アイコンの幅 px)と、線の太さ(px)
  const SIZE = { lg: { w: 160, stroke: 3 }, md: { w: 64, stroke: 2 }, sm: { w: 40, stroke: 1.5 }, xs: { w: 24, stroke: 1.25 } };
  const n2 = (v) => Math.round(v * 100) / 100;

  // SVG の Q(2 次ベジエ)・T(その反転)を、C(3 次ベジエ)へ、同じ曲線のまま変換する(パスは M・C だけにするため。上のコメント参照)
  const quadToCubic = (d) => {
    const t = d.match(/[MQTZ]|-?[\d.]+/g);
    let i = 0, x = 0, y = 0, qx = 0, qy = 0, out = '';
    const num = () => parseFloat(t[i++]);
    while (i < t.length) {
      const c = t[i++];
      if (c === 'M') { x = num(); y = num(); out += `M ${n2(x)} ${n2(y)}`; }
      else if (c === 'Q' || c === 'T') {
        const cx = c === 'Q' ? num() : 2 * x - qx, cy = c === 'Q' ? num() : 2 * y - qy; // T は、直前の Q の制御点を、いまの始点で反転する
        const ex = num(), ey = num();
        out += ` C ${n2(x + (2 / 3) * (cx - x))} ${n2(y + (2 / 3) * (cy - y))} ${n2(ex + (2 / 3) * (cx - ex))} ${n2(ey + (2 / 3) * (cy - ey))} ${n2(ex)} ${n2(ey)}`;
        qx = cx; qy = cy; x = ex; y = ey;
      } else if (c === 'Z') out += ' Z';
    }
    return out;
  };

  // 下から重ねる順。unit は案 A で「何点目で灯るか」(1〜5)。y は、水位・案 A と比べる代表の Y 座標(パスの始点。下ほど大きい)
  const PARTS = [
    { id: '下のバンズ', unit: 1, y: 82, color: COLOR.bun, d: 'M 12 82 C 12 94 30 98 60 98 C 90 98 108 94 108 82' },
    { id: 'パティ', unit: 2, y: 76, color: COLOR.patty, d: quadToCubic('M 9 76 Q 28 70 47 76 T 85 76 T 111 76') },
    { id: 'チーズ', unit: 3, y: 63, color: COLOR.cheese, d: quadToCubic('M 10 63 Q 26 58 42 63 T 74 63 T 110 63') },
    { id: 'トマト', unit: 4, y: 50, color: COLOR.tomato, d: quadToCubic('M 11 50 Q 27 45 43 50 T 77 50 T 109 50') },
    { id: 'レタス', unit: 4, y: 37, color: COLOR.lettuce, d: quadToCubic('M 6 37 Q 16 28 26 37 Q 36 28 46 37 Q 56 28 66 37 Q 76 28 86 37 Q 96 28 106 37 Q 112 33 114 37') },
    { id: '上のバンズ', unit: 5, y: 34, color: COLOR.bun, d: 'M 14 34 C 14 12 30 2 60 2 C 90 2 106 12 106 34' },
  ];

  // パスの外接の四角(曲線を刻んで測る)。Penpot の図形のジオメトリに使う
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
  PARTS.forEach((p) => { p.bbox = bboxOf(p.d); });
  // 極小(24px)だけの簡略版: 6 本の線は隙間が潰れて見分けられないため、3 本(下のバンズ・チーズ(具の代表)・上のバンズ)に間引く。
  // 同じパス・色をそのまま使い、線を新しく作らない(パティ・トマト・レタスの線を抜くだけ)
  const PARTS_XS = PARTS.filter((p) => ['下のバンズ', 'チーズ', '上のバンズ'].includes(p.id));

  // 白黒にしたときの色(CSS の grayscale と同じ係数)
  const grayOf = (hex) => { const v = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16)); const g = Math.round(0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2]); return '#' + g.toString(16).padStart(2, '0').repeat(3); };

  // 評価と大きさから、描く線の一覧を作る。食材ごとに、点灯(その食材の色)/ 消灯(EMPTY)のどちらかを選ぶだけ(塗りは無し)
  // variant: 'B' 水位(連続値) / 'A' 部品ごと(unit の段階)。value は、数字に出す値(1 桁に丸めた値)。max は評価の最大値
  const resolve = ({ variant = 'B', value = 0, max = 5, size = 'md', gray = false }) => {
    const sz = SIZE[size], scale = sz.w / VB.w, sw = n2(sz.stroke / scale);
    const tone = (c) => (gray ? grayOf(c) : c);
    const level = BOTTOM - (BOTTOM - TOP) * (Math.max(0, Math.min(value, max)) / max); // 水位の y(これ以上(下)の食材が点灯)
    const unitValue = (value / max) * 5; // 案 A は 5 段。最大値が違っても 5 段の割合にする
    const shapes = (size === 'xs' ? PARTS_XS : PARTS).map((p) => {
      const lit = variant === 'A' ? unitValue >= p.unit : p.y >= level;
      return { part: p.id, d: p.d, fill: null, stroke: tone(lit ? p.color : EMPTY), sw, bbox: p.bbox, round: true };
    });
    return { vb: [VB.w, VB.h], width: sz.w, height: n2(sz.w * VB.h / VB.w), shapes };
  };

  const svgOf = (icon) => {
    let body = '';
    icon.shapes.forEach((s) => { body += `<path d="${s.d}" fill="none" stroke="${s.stroke}" stroke-width="${s.sw}" stroke-linecap="round" stroke-linejoin="round"/>`; });
    return `<svg viewBox="0 0 ${icon.vb[0]} ${icon.vb[1]}" width="${icon.width}" height="${icon.height}" xmlns="http://www.w3.org/2000/svg">${body}</svg>`;
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

  window.Burger = { VB, SIZE, COLOR, EMPTY, PARTS, resolve, svgOf, mount, grayOf };
})();
