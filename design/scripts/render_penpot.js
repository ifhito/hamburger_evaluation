// Penpot に作った画面を、Penpot の閲覧画面(ビューアー)で開いて、画像にする。
// 実際の画面のスクリーンショット(capture.js が保存)と、見た目を並べて比べるために使う。
// 使い方は design/README.md の「見た目の確認」を参照。
const { chromium } = require('playwright');
const fs = require('fs');

const BASE = process.env.PENPOT_URL || 'http://localhost:9001';
// Penpot の画面が、自分の URL として持っている値(PENPOT_PUBLIC_URI)。BASE と違うとき(コンテナの中から開くとき)は、その宛先を BASE に読み替える。
const PUBLIC = process.env.PENPOT_PUBLIC_URL || BASE;
const EMAIL = process.env.PENPOT_EMAIL;
const PASSWORD = fs.readFileSync(process.env.PENPOT_PASSWORD_FILE, 'utf8').trim();
const MAP = JSON.parse(fs.readFileSync(process.env.MAP_JSON, 'utf8'));
const OUT = process.env.OUT || '/work/render';
const ONLY = process.env.ONLY ? process.env.ONLY.split(',') : null; // 例: signin-pc,shops-mobile
// CLIP=1 のとき、画面の枠だけを切り出した JPEG にする(リデザインのラフの画像用)。ビューアーで 100% にしたとき、枠の左上は (100, 124)
const CLIP = process.env.CLIP === '1';

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1400, height: 900 }, locale: 'en-US' });
  if (PUBLIC !== BASE) {
    await ctx.route(`${PUBLIC}/**`, async (route) => {
      const res = await route.fetch({ url: route.request().url().replace(PUBLIC, BASE) });
      await route.fulfill({ response: res });
    });
  }
  const login = await ctx.request.post(`${BASE}/api/rpc/command/login-with-password`, { data: { email: EMAIL, password: PASSWORD }, headers: { Accept: 'application/json' } });
  if (!login.ok()) throw new Error('Penpot にログインできません: ' + login.status());
  // 枠の id は、ファイルの中身から名前で引く
  const file = await (await ctx.request.post(`${BASE}/api/rpc/command/get-file`, { data: { id: MAP.fileId }, headers: { Accept: 'application/json' } })).json();
  const done = [];
  for (const s of MAP.screens) {
    const key = `${s.key}-${s.size}`;
    if (ONLY && !ONLY.includes(key)) continue;
    const pageId = MAP.pages[s.page];
    const objs = file.data.pagesIndex[pageId].objects;
    const frame = Object.values(objs).find((o) => o.type === 'frame' && o.name === s.frame);
    if (!frame) { console.error('枠が見つかりません: ' + s.frame); continue; }
    const page = await ctx.newPage();
    await page.setViewportSize({ width: Math.ceil(frame.width) + 200, height: Math.ceil(frame.height) + 200 });
    await page.goto(`${BASE}/#/view?file-id=${MAP.fileId}&page-id=${pageId}&section=interactions&frame-id=${frame.id}&index=0`, { waitUntil: 'networkidle' });
    await page.waitForTimeout(2500);
    await page.keyboard.press('Shift+0'); // 表示を 100% にする
    await page.waitForTimeout(800);
    if (CLIP) await page.screenshot({ path: `${OUT}/${key}.jpg`, type: 'jpeg', quality: Number(process.env.QUALITY || 80), clip: { x: 100, y: 124, width: Math.ceil(frame.width), height: Math.ceil(frame.height) } });
    else await page.screenshot({ path: `${OUT}/${key}.png` });
    done.push(key);
    await page.close();
  }
  await browser.close();
  console.log(JSON.stringify(done));
})();
