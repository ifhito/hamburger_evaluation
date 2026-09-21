// リデザインの見本(design/redesign/*.html)を、ヘッドレスのブラウザで開き、見た目を長方形と文字の一覧(JSON)にする。
// build_redesign.py が、この JSON から Penpot のデザインを作る。使い方は design/README.md の「リデザインのラフ」を参照。
const { chromium } = require('playwright');
const fs = require('fs');
const extract = require('./extract');

const MOCK_DIR = process.env.MOCK_DIR || '/mock'; // design/redesign
const OUT = process.env.OUT || '/work/out-redesign';

// [キー, 画面の名前, HTML, 言語]。PC(1280)とモバイル(375)を、それぞれ取る
const SCREENS = [
  { key: 'shops', title: 'ショップ一覧', file: 'shops.html', lang: 'ja' },
  { key: 'reviews', title: 'レビュー一覧', file: 'reviews.html', lang: 'ja' },
  { key: 'review-detail', title: 'レビュー詳細', file: 'review-detail.html', lang: 'ja' },
  { key: 'shops-en', title: 'ショップ一覧(English)', file: 'shops.html', lang: 'en' },
];
const SIZES = [{ key: 'pc', w: 1280, h: 800 }, { key: 'mobile', w: 375, h: 812 }];

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const result = [];
  for (const size of SIZES) {
    for (const sc of SCREENS) {
      const ctx = await browser.newContext({ viewport: { width: size.w, height: size.h }, locale: sc.lang === 'en' ? 'en-US' : 'ja-JP' });
      const page = await ctx.newPage();
      await page.goto(`file://${MOCK_DIR}/${sc.file}?lang=${sc.lang}`, { waitUntil: 'networkidle' });
      await page.evaluate(() => document.fonts.ready);
      const font = await page.evaluate(() => document.fonts.check('16px "Noto Sans JP"'));
      await page.waitForTimeout(300);
      const data = await page.evaluate(extract);
      const name = `${sc.key}-${size.key}`;
      fs.writeFileSync(`${OUT}/${name}.json`, JSON.stringify({ key: sc.key, title: sc.title, size: size.key, viewport: size.w, url: sc.file, ...data }));
      await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true });
      result.push({ name, nodes: data.nodes.length, w: data.width, h: data.height, notoLoaded: font });
      await ctx.close();
    }
  }
  // 値のページ(色・コントラスト・文字)
  const ctx = await browser.newContext({ viewport: { width: 1320, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(`file://${MOCK_DIR}/tokens.html`, { waitUntil: 'networkidle' });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
  const data = await page.evaluate(extract);
  const tokens = await page.evaluate(() => ({ tokens: window.__tokens, contrast: window.__contrast }));
  fs.writeFileSync(`${OUT}/tokens.json`, JSON.stringify({ key: 'tokens', title: 'デザインの値', size: 'pc', viewport: 1320, url: 'tokens.html', ...data }));
  fs.writeFileSync(`${OUT}/tokens-values.json`, JSON.stringify(tokens, null, 1));
  await page.screenshot({ path: `${OUT}/tokens.png`, fullPage: true });
  await browser.close();
  console.log(JSON.stringify(result));
  console.log(JSON.stringify(tokens.contrast.map((c) => `${c.pass ? 'OK ' : 'NG '}${c.ratio}:1 ${c.label}`), null, 1));
})();
