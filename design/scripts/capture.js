// いまの画面(実際の frontend)を、ヘッドレスのブラウザで開き、見た目を「長方形」と「文字」の一覧(JSON)と、スクリーンショットにして書き出す。
// build_penpot.py が、この JSON から Penpot のデザインを作る。使い方は design/README.md の「いまの画面から作り直す」を参照。
// 画面ごとの見え方(ログイン済みか・管理者か)は、下の routes に書いてある。画面を足したときは、routes と build_penpot.py の SCREEN_ORDER を足す。
const { chromium } = require('playwright');
const fs = require('fs');

const BASE = process.env.BASE || 'http://localhost:5173'; // frontend の URL(API は BASE + '/api')
const OUT = process.env.OUT || '/work/out'; // 書き出し先
// 開発用のサンプルデータ(backend-go/cmd/seed)のユーザー。パスワードはその devPassword と同じ
const PW = process.env.SEED_PASSWORD || 'Password123!';
const USER_EMAIL = process.env.USER_EMAIL || 'alice@example.com';
const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@example.com';

async function api(path, opts = {}) {
  const r = await fetch(BASE + '/api' + path, {
    headers: { 'Content-Type': 'application/json', ...(opts.token ? { Authorization: 'Bearer ' + opts.token } : {}) },
    method: opts.method || 'GET',
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  return r.json();
}

// ページの中で実行する: 見えている要素を、長方形と文字の一覧にする。
function extract() {
  const nodes = [];
  const sx = window.scrollX, sy = window.scrollY;
  const rgba = (c) => { const m = c && c.match(/rgba?\(([^)]+)\)/); if (!m) return null; const p = m[1].split(',').map(parseFloat); return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 }; };
  const hex = (c) => '#' + [c.r, c.g, c.b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');
  const px = (v) => parseFloat(v) || 0;
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
    const bw = px(cs.borderTopWidth);
    const hasBorder = bw > 0 && cs.borderTopStyle !== 'none' && (rgba(cs.borderTopColor) || { a: 0 }).a > 0;
    if (tag !== 'html' && tag !== 'body' && ((bg && bg.a > 0) || hasBorder)) {
      nodes.push({
        kind: 'rect', tag, name: cls(el) || tag, ctl: g, x: r.left + sx, y: r.top + sy, w: r.width, h: r.height,
        fill: bg && bg.a > 0 ? hex(bg) : null, fillOpacity: bg ? bg.a : 0,
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
        const ty = r.top + sy + pt + Math.max(0, (th - ts.size * 1.2) / 2);
        nodes.push({
          kind: 'text', tag, name: cls(el) || tag, ctl: g, text,
          x: r.left + sx + pl, y: ty, w: tw, h: ts.size * 1.2, style: ts,
          lines: [{ text, x: r.left + sx + pl, y: ty, w: tw, h: ts.size * 1.2 }],
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
}

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const alice = await api('/login', { method: 'POST', body: { email: USER_EMAIL, password: PW } });
  const admin = await api('/login', { method: 'POST', body: { email: ADMIN_EMAIL, password: PW } });
  const shops = await api('/shops');
  const shop = shops.find((s) => s.status === 'active') || shops[0];
  const reviews = await api('/reviews', { token: alice.token });
  const own = reviews.find((r) => r.user && r.user.id === alice.id) || reviews[0];
  const adminShops = await api('/admin/shops', { token: admin.token });
  const pending = adminShops.find((s) => s.status === 'pending') || adminShops[0];
  const routes = [
    { key: 'signin', title: 'サインイン', path: '/signin', who: null },
    { key: 'signin-error', title: 'サインイン(エラー表示)', path: '/signin', who: null, action: 'badlogin' },
    { key: 'signup', title: '新規登録', path: '/signup', who: null },
    { key: 'signup-sent', title: 'メールを確認してください', path: '/signup', who: null, action: 'signup' },
    { key: 'shops', title: 'ショップの一覧', path: '/shops', who: alice },
    { key: 'shop-detail', title: 'ショップの詳細', path: '/shops/' + shop.id, who: alice },
    { key: 'reviews', title: 'レビューの一覧', path: '/reviews', who: alice },
    { key: 'review-detail', title: 'レビューの詳細', path: '/reviews/' + own.id, who: alice },
    { key: 'review-new', title: 'レビューの投稿', path: '/reviews/new?shop_id=' + shop.id, who: alice },
    { key: 'review-edit', title: 'レビューの編集', path: '/reviews/' + own.id + '/edit', who: alice },
    { key: 'profile', title: 'プロフィール', path: '/users/' + alice.id, who: alice },
    { key: 'profile-edit', title: 'プロフィールの編集', path: '/users/' + alice.id + '/edit', who: alice },
    { key: 'admin-shops', title: '管理(ショップの承認・却下)', path: '/admin/shops', who: admin },
    { key: 'admin-shop-edit', title: '管理(ショップの編集)', path: '/admin/shops/' + pending.id + '/edit', who: admin },
  ];
  const browser = await chromium.launch();
  const sizes = [{ key: 'pc', w: 1280, h: 800 }, { key: 'mobile', w: 375, h: 812 }];
  const result = [];
  for (const size of sizes) {
    for (const rt of routes) {
      const ctx = await browser.newContext({ viewport: { width: size.w, height: size.h }, locale: 'en-US' });
      if (rt.who) {
        const u = { id: rt.who.id, username: rt.who.username, email: rt.who.email, admin: rt.who.admin, canModerate: rt.who.canModerate ?? rt.who.can_moderate ?? rt.who.admin };
        await ctx.addInitScript(([t, user]) => { localStorage.setItem('token', t); localStorage.setItem('auth_user', JSON.stringify(user)); }, [rt.who.token, u]);
      }
      const page = await ctx.newPage();
      await page.goto(BASE + rt.path, { waitUntil: 'networkidle' });
      if (rt.action === 'signup') {
        const id = Date.now();
        await page.fill('#username', 'newuser' + id).catch(() => {});
        await page.fill('#email', `newuser${id}@example.com`).catch(() => {});
        await page.fill('#password', PW).catch(() => {});
        await page.fill('#passwordConfirmation', PW).catch(() => {});
        await page.click('button[type=submit]');
        await page.waitForTimeout(1500);
      }
      if (rt.action === 'badlogin') {
        await page.fill('#email', USER_EMAIL);
        await page.fill('#password', 'Wrongpass1!');
        await page.click('button[type=submit]');
        await page.waitForTimeout(1200);
      }
      await page.waitForTimeout(600);
      const data = await page.evaluate(extract);
      const name = `${rt.key}-${size.key}`;
      fs.writeFileSync(`${OUT}/${name}.json`, JSON.stringify({ key: rt.key, title: rt.title, size: size.key, viewport: size.w, url: page.url(), ...data }));
      await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true });
      result.push({ name, nodes: data.nodes.length, w: data.width, h: data.height, url: page.url().replace(BASE, '') });
      await ctx.close();
    }
  }
  await browser.close();
  console.log(JSON.stringify(result));
})();
