// 実際の画面(左)と、Penpot の画面(右)を並べて、1 枚の画像にする。
// 実際の画面は capture.js が、Penpot の画面は render_penpot.js が、それぞれ画像にしたもの。
// 使い方は design/README.md の「見た目の確認」を参照。
const { chromium } = require('playwright');
const fs = require('fs');

const CAPTURE_DIR = process.env.CAPTURE_DIR || '/work/out'; // capture.js の出力(<画面>-<pc|mobile>.json と .png)
const RENDER_DIR = process.env.RENDER_DIR || '/work/render'; // render_penpot.js の出力
const OUT = process.env.OUT || '/work/compare';
const MAP = JSON.parse(fs.readFileSync(process.env.MAP_JSON || '/work/map.json', 'utf8'));
const ONLY = process.env.ONLY ? process.env.ONLY.split(',') : null; // 例: signin-pc,shops-mobile
const FORMAT = process.env.FORMAT === 'jpeg' ? 'jpeg' : 'png';
// ビューアーで枠を 100% 表示したときの、枠の左上の位置(画像の余白を切り取るために使う)
const OFF_X = 100, OFF_Y = 124;

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const dataUri = (file) => 'data:image/png;base64,' + fs.readFileSync(file).toString('base64');
  for (const s of MAP.screens) {
    const key = `${s.key}-${s.size}`;
    if (ONLY && !ONLY.includes(key)) continue;
    const meta = JSON.parse(fs.readFileSync(`${CAPTURE_DIR}/${key}.json`, 'utf8'));
    const w = meta.viewport, h = meta.height;
    await page.setViewportSize({ width: w * 2 + 60, height: h + 70 });
    await page.setContent(`<body style="margin:0;background:#888;font:14px sans-serif;color:#fff">
      <div style="display:flex;gap:20px;padding:10px 20px 0">
        <div><div style="height:26px">実際の画面(${meta.title} / ${s.size})</div><img src="${dataUri(`${CAPTURE_DIR}/${key}.png`)}" style="display:block;width:${w}px;height:${h}px"></div>
        <div><div style="height:26px">Penpot</div><div style="width:${w}px;height:${h}px;overflow:hidden"><img src="${dataUri(`${RENDER_DIR}/${key}.png`)}" style="display:block;margin:-${OFF_Y}px 0 0 -${OFF_X}px"></div></div>
      </div></body>`);
    await page.screenshot({ path: `${OUT}/${key}.${FORMAT === 'jpeg' ? 'jpg' : 'png'}`, type: FORMAT, ...(FORMAT === 'jpeg' ? { quality: 70 } : {}) });
  }
  await browser.close();
})();
