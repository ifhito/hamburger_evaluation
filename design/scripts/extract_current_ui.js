// 実際のReact画面のDOMを読み取り専用で計測する。CUAのevaluateに渡す。写真・SVG・文字の実測行位置を保持する。
// ページの中で実行する: 見えている要素を、長方形と文字の一覧にする。
module.exports = function extract() {
  const nodes = [];
  const sx = window.scrollX, sy = window.scrollY;
  const rgba = (c) => { const m = c && c.match(/rgba?\(([^)]+)\)/); if (!m) return null; const p = m[1].split(',').map(Number.parseFloat); return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 }; };
  const hex = (c) => '#' + [c.r, c.g, c.b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');
  const px = (v) => Number.parseFloat(v) || 0;
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
    if (/^-?[\d.]+deg$/.test(parts[0])) angle = Number.parseFloat(parts.shift());
    const stops = parts.map((p, i) => {
      const mm = p.match(/^(rgba?\([^)]*\))\s*([\d.]+)?%?$/);
      if (!mm) return null;
      const c = rgba(mm[1]);
      return { color: hex(c), opacity: c.a, offset: mm[2] !== undefined ? Number.parseFloat(mm[2]) / 100 : i / Math.max(1, parts.length - 1) };
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
  // Native input values have no text node/Range. Use font size only for their text run bounds.
  const measure = (text, cs) => Array.from(text).reduce((w,ch)=>w+(ch.codePointAt(0)>0x2e7f?1:.55)*px(cs.fontSize),0);
  const fontHeight = (cs) => px(cs.fontSize)*1.45;
  const walk = (el, group) => {
    const cs = getComputedStyle(el);
    if (!visible(cs)) return;
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) return;
    const tag = el.tagName.toLowerCase();
    if (tag === 'script' || tag === 'style' || tag === 'option') return;
    // Reactが表示したSVGを、形状・色・線幅を変えずに取り込む。
    if (tag === 'svg') {
      const rawVb=(el.getAttribute('viewBox') || '0 0 120 100').split(/[,\s]+/).map(Number); const vb={x:rawVb[0],y:rawVb[1],width:rawVb[2],height:rawVb[3]};
      const shapes = Array.from(el.querySelectorAll('path')).map(path => {
        const ps = getComputedStyle(path), b = vb;
        return { part: 'SVG path', d: path.getAttribute('d'), fill: ps.fill === 'none' ? null : {color: hex(rgba(ps.fill))},
          stroke: ps.stroke === 'none' ? '#000000' : hex(rgba(ps.stroke)), strokeNone: ps.stroke === 'none', sw: px(ps.strokeWidth),
          dash: ps.strokeDasharray === 'none' ? null : ps.strokeDasharray, round: ps.strokeLinecap === 'round', bbox: [b.x,b.y,b.width,b.height] };
      });
      if (shapes.length) nodes.push({kind:'icon',tag,name:el.closest('[role="img"]')?.getAttribute('aria-label') || 'サービスロゴ',ctl:group,
        x:r.left+sx,y:r.top+sy,w:r.width,h:r.height,icon:{vb:[vb.width,vb.height],origin:[vb.x,vb.y],shapes}});
      return;
    }
    const isControl = ['button', 'input', 'textarea', 'select'].includes(tag);
    const g = isControl ? ++ctl : group;
    const bg = rgba(cs.backgroundColor);
    const grad = gradientOf(cs.backgroundImage);
    const bw = px(cs.borderTopWidth);
    const sides = ['Top', 'Right', 'Bottom', 'Left'].map(side => ({
      width: px(cs['border'+side+'Width']), style: cs['border'+side+'Style'], color: rgba(cs['border'+side+'Color'])
    }));
    const hasBorder = sides.some(b => b.width > 0 && b.style !== 'none' && b.color?.a > 0);
    const uniformBorder = hasBorder && sides.every(b => JSON.stringify(b) === JSON.stringify(sides[0]));
    const clippedRadius = () => {
      const radii = [px(cs.borderTopLeftRadius), px(cs.borderTopRightRadius), px(cs.borderBottomRightRadius), px(cs.borderBottomLeftRadius)];
      for (let p=el.parentElement; p && p!==document.body; p=p.parentElement) {
        const pc=getComputedStyle(p), pr=p.getBoundingClientRect();
        if (pc.overflow==='hidden' && Math.abs(pr.top-r.top)<2 && Math.abs(pr.width-r.width)<3) {
          radii[0]=Math.max(radii[0],px(pc.borderTopLeftRadius));
          radii[1]=Math.max(radii[1],px(pc.borderTopRightRadius)); break;
        }
      }
      return radii;
    };
    if (tag !== 'html' && tag !== 'body' && ((bg && bg.a > 0) || hasBorder || grad)) {
      nodes.push({
        kind: 'rect', tag, name: cls(el) || tag, ctl: g, x: r.left + sx, y: r.top + sy, w: r.width, h: r.height,
        fill: bg && bg.a > 0 && !grad ? hex(bg) : null, fillOpacity: bg ? bg.a : 0, gradient: grad,
        stroke: uniformBorder ? { w: bw, color: hex(sides[0].color), opacity: sides[0].color.a } : null,
        radius: clippedRadius(),
      });
      if (hasBorder && !uniformBorder) sides.forEach((b,i) => {
        if (!b.width || b.style==='none' || !b.color?.a) return;
        const positions = [[r.left,r.top,r.width,b.width],[r.right-b.width,r.top,b.width,r.height],
          [r.left,r.bottom-b.width,r.width,b.width],[r.left,r.top,b.width,r.height]];
        const [x,y,w,h]=positions[i];
        nodes.push({kind:'rect',tag,name:'区切り線',ctl:g,x:x+sx,y:y+sy,w,h,
          fill:hex(b.color),fillOpacity:b.color.a,stroke:null,radius:[0,0,0,0]});
      });
    }
    if (tag === 'img') {
      const radius=clippedRadius();
      nodes.push({kind:'image',tag,name:el.alt || '画像',ctl:group,x:r.left+sx,y:r.top+sy,w:r.width,h:r.height,
        src:el.currentSrc,naturalWidth:el.naturalWidth,naturalHeight:el.naturalHeight,objectFit:cs.objectFit,radius});
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
      if (tag === 'select' && cs.appearance !== 'none') {
        nodes.push({kind:'icon',tag,name:'並び替えの選択矢印',ctl:g,x:r.right+sx-13,y:r.top+sy+r.height/2-2,w:8,h:4,
          icon:{vb:[8,4],origin:[0,0],shapes:[{part:'選択矢印',d:'M 0 0 L 4 4 L 8 0',fill:null,stroke:ts.color,strokeNone:false,sw:1,dash:null,round:true}]}});
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
