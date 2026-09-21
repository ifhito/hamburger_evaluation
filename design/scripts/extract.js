// 画面(HTML)の見た目を、長方形と文字の一覧(JSON)にする。ページの中で page.evaluate に渡して実行する(自己完結した関数)。
// capture.js(いまの画面)と capture_mock.js(リデザインの見本)が使う。
// ページの中で実行する: 見えている要素を、長方形と文字の一覧にする。
module.exports = function extract() {
  const nodes = [];
  const sx = window.scrollX, sy = window.scrollY;
  const rgba = (c) => { const m = c && c.match(/rgba?\(([^)]+)\)/); if (!m) return null; const p = m[1].split(',').map(parseFloat); return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 }; };
  const hex = (c) => '#' + [c.r, c.g, c.b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');
  const px = (v) => parseFloat(v) || 0;
  // background-image の linear-gradient(...) を、角度と色の止まり位置にする。見本の HTML は、角度(deg)と % を必ず書く
  const gradientOf = (bgImage) => {
    const m = bgImage && bgImage.match(/^linear-gradient\((.*)\)$/);
    if (!m) return null;
    const parts = [];
    let depth = 0, cur = '';
    for (const ch of m[1]) {
      if (ch === '(') depth++;
      if (ch === ')') depth--;
      if (ch === ',' && depth === 0) { parts.push(cur.trim()); cur = ''; } else cur += ch;
    }
    parts.push(cur.trim());
    let angle = 180;
    if (/^-?[\d.]+deg$/.test(parts[0])) angle = parseFloat(parts.shift());
    const stops = parts.map((p, i) => {
      const mm = p.match(/^(rgba?\([^)]*\))\s*([\d.]+)?%?$/);
      if (!mm) return null;
      const c = rgba(mm[1]);
      return { color: hex(c), opacity: c.a, offset: mm[2] !== undefined ? parseFloat(mm[2]) / 100 : i / Math.max(1, parts.length - 1) };
    });
    return stops.every(Boolean) ? { angle, stops } : null;
  };
  let ctl = 0;
  const cls = (el) => { const c = (el.getAttribute('class') || '').split(' ')[0] || ''; const m = c.match(/^_?([A-Za-z0-9]+?)_/); return m ? m[1] : c; };
  const visible = (cs) => cs.display !== 'none' && cs.visibility !== 'hidden' && cs.opacity !== '0';
  const textStyle = (cs) => ({
    family: cs.fontFamily.split(',')[0].replace(/["']/g, '').trim(),
    size: px(cs.fontSize),
    weight: cs.fontWeight,
    color: hex(rgba(cs.color) || { r: 0, g: 0, b: 0, a: 1 }),
    lineHeight: cs.lineHeight === 'normal' ? null : px(cs.lineHeight),
    align: cs.textAlign,
    letterSpacing: cs.letterSpacing === 'normal' ? 0 : px(cs.letterSpacing),
    underline: cs.textDecorationLine.includes('underline'),
  });
  // 文字を 1 つずつ調べて、ブラウザの折り返しで実際にできた行(文字列と位置)を求める。
  const linesOf = (node) => {
    const txt = node.textContent;
    const out = [];
    let cur = null;
    let i = 0;
    for (const ch of txt) {
      const len = ch.length;
      const range = document.createRange();
      range.setStart(node, i);
      range.setEnd(node, i + len);
      const rs = Array.from(range.getClientRects()).filter((q) => q.width > 0 && q.height > 0);
      i += len;
      if (!rs.length) continue;
      const q = rs[0];
      if (cur && Math.abs(q.top - cur.top) < 3) { cur.text += ch; cur.right = Math.max(cur.right, q.right); cur.bottom = Math.max(cur.bottom, q.bottom); }
      else { cur = { top: q.top, left: q.left, right: q.right, bottom: q.bottom, text: ch }; out.push(cur); }
    }
    return out.map((l) => ({ text: l.text.replace(/\s+$/, ''), x: l.left + sx, y: l.top + sy, w: l.right - l.left, h: l.bottom - l.top })).filter((l) => l.text);
  };
  const measure = (text, cs) => { const c = document.createElement('canvas').getContext('2d'); c.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`; return c.measureText(text).width; };
  // 1 行の高さ(フォントの上端から下端まで)。Range の矩形と同じ高さで、行の中の文字の置き場所を、入力欄でも同じ式で求めるために使う
  const fontHeight = (cs) => { const c = document.createElement('canvas').getContext('2d'); c.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`; const m = c.measureText('あ'); return m.fontBoundingBoxAscent + m.fontBoundingBoxDescent; };
  const walk = (el, group) => {
    const cs = getComputedStyle(el);
    if (!visible(cs)) return;
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) return;
    const tag = el.tagName.toLowerCase();
    if (tag === 'script' || tag === 'style' || tag === 'option') return;
    const isControl = ['button', 'input', 'textarea', 'select'].includes(tag);
    const g = isControl ? ++ctl : group;
    const bg = rgba(cs.backgroundColor);
    const grad = gradientOf(cs.backgroundImage);
    const bw = px(cs.borderTopWidth);
    const hasBorder = bw > 0 && cs.borderTopStyle !== 'none' && (rgba(cs.borderTopColor) || { a: 0 }).a > 0;
    if (tag !== 'html' && tag !== 'body' && ((bg && bg.a > 0) || hasBorder || grad)) {
      nodes.push({
        kind: 'rect', tag, name: cls(el) || tag, ctl: g, x: r.left + sx, y: r.top + sy, w: r.width, h: r.height,
        fill: bg && bg.a > 0 && !grad ? hex(bg) : null, fillOpacity: bg ? bg.a : 0, gradient: grad,
        stroke: hasBorder ? { w: bw, color: hex(rgba(cs.borderTopColor)), opacity: rgba(cs.borderTopColor).a } : null,
        radius: [px(cs.borderTopLeftRadius), px(cs.borderTopRightRadius), px(cs.borderBottomRightRadius), px(cs.borderBottomLeftRadius)],
      });
    }
    if (tag === 'img') {
      nodes.push({ kind: 'rect', tag, name: 'image', ctl: group, x: r.left + sx, y: r.top + sy, w: r.width, h: r.height, fill: '#e5e7eb', fillOpacity: 1, stroke: null, radius: [0, 0, 0, 0] });
      return;
    }
    if (tag === 'input' || tag === 'textarea' || tag === 'select') {
      let text = '';
      if (tag === 'select') text = el.options[el.selectedIndex] ? el.options[el.selectedIndex].text : '';
      else if (el.type === 'password' && el.value) text = '•'.repeat(el.value.length);
      else text = el.value || '';
      const ts = textStyle(cs);
      if (!text && el.placeholder) {
        text = el.placeholder;
        const pc = rgba(getComputedStyle(el, '::placeholder').color);
        if (pc) ts.color = hex(pc);
      }
      if (text) {
        const pl = px(cs.paddingLeft) + bw, pt = px(cs.paddingTop) + bw;
        const tw = Math.min(measure(text, cs), Math.max(4, r.width - pl - px(cs.paddingRight) - bw));
        const th = Math.max(ts.size, r.height - pt - px(cs.paddingBottom) - bw);
        const fh = fontHeight(cs) || ts.size * 1.2;
        const ty = r.top + sy + pt + Math.max(0, (th - fh) / 2);
        nodes.push({
          kind: 'text', tag, name: cls(el) || tag, ctl: g, text,
          x: r.left + sx + pl, y: ty, w: tw, h: fh, style: ts,
          lines: [{ text, x: r.left + sx + pl, y: ty, w: tw, h: fh }],
        });
      }
      return;
    }
    for (const ch of el.childNodes) {
      if (ch.nodeType === 3) {
        const t = ch.textContent.replace(/\s+/g, ' ').trim();
        if (!t) continue;
        const range = document.createRange();
        range.selectNodeContents(ch);
        const rects = Array.from(range.getClientRects()).filter((q) => q.width > 0 && q.height > 0);
        if (!rects.length) continue;
        const x1 = Math.min(...rects.map((q) => q.left)), y1 = Math.min(...rects.map((q) => q.top));
        const x2 = Math.max(...rects.map((q) => q.right)), y2 = Math.max(...rects.map((q) => q.bottom));
        nodes.push({ kind: 'text', tag, name: cls(el) || tag, ctl: g, text: t, x: x1 + sx, y: y1 + sy, w: x2 - x1, h: y2 - y1, style: textStyle(cs), lines: linesOf(ch) });
      } else if (ch.nodeType === 1) walk(ch, g);
    }
  };
  walk(document.body, 0);
  const bs = getComputedStyle(document.body);
  return { width: Math.ceil(document.documentElement.scrollWidth), height: Math.ceil(document.documentElement.scrollHeight), bg: hex(rgba(bs.backgroundColor) || { r: 255, g: 255, b: 255, a: 1 }), nodes };
};
