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

const extract = require('./extract');

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
