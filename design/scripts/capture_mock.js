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
  { key: 'states', title: '空・読み込み・エラー・404', file: 'states.html', lang: 'ja' },
  { key: 'states-en', title: '空・読み込み・エラー・404(English)', file: 'states.html', lang: 'en' },
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
  // 評価のバーガーの比較のページ(案 B と案 A、大きさ 4 つ、色と白黒。アイコンは、パスの一覧として取る)
  const ictx = await browser.newContext({ viewport: { width: 1520, height: 900 } });
  const ipage = await ictx.newPage();
  await ipage.goto(`file://${MOCK_DIR}/rating-icons.html`, { waitUntil: 'networkidle' });
  await ipage.evaluate(() => document.fonts.ready);
  await ipage.waitForTimeout(300);
  const idata = await ipage.evaluate(extract);
  fs.writeFileSync(`${OUT}/rating-icons.json`, JSON.stringify({ key: 'rating-icons', title: '評価のバーガーの比較', size: 'pc', viewport: 1520, url: 'rating-icons.html', ...idata }));
  await ipage.screenshot({ path: `${OUT}/rating-icons.png`, fullPage: true });
  // 評価のバーガーの自己確認: 評価 0 は全部白抜き、5 は水位の境目なし、2.5 は途中の部品に水位の境目がある。壊れたら、ここで止める
  const chk = await ipage.evaluate(() => {
    const r = (v, size = 'md') => window.Burger.resolve({ variant: 'B', value: v, size }).shapes;
    return { empty0: r(0).every((s) => s.fill && s.fill.color === '#ffffff'), full5: r(5).every((s) => !(s.fill && s.fill.axis)), mid: r(2.5).some((s) => s.fill && s.fill.axis === 'y'), simple: [r(3, 'sm').length, r(3, 'xs').length] };
  });
  if (!chk.empty0 || !chk.full5 || !chk.mid) throw new Error('評価のバーガーの自己確認に失敗: ' + JSON.stringify(chk));
  await browser.close();
  console.log(JSON.stringify(result));
  console.log(JSON.stringify(tokens.contrast.map((c) => `${c.used === false ? '不使用' : c.used === 'ref' ? '参考 ' : c.pass ? 'OK ' : 'NG '}${c.ratio}:1 ${c.label}`), null, 1));
})();
