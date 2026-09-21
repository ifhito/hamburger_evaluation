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
// 第 1 弾以降の画面。[グループ(Penpot のページ名の元), キー, 画面の名前, HTML, state, 追加の query(省略可。例 'auth=out&user=bob'), 言語(省略可。既定は ja と en の両方)]。
// state は、HTML の data-only の切り替え(?state=)。英語版は、キーの末尾に -en を付けて、自動で足す
const GROUPED = [
  ['認証', 'signin', 'サインイン', 'signin.html', 'default'],
  ['認証', 'signin-error', 'サインイン(エラー)', 'signin.html', 'error'],
  ['認証', 'signup', '新規登録', 'signup.html', 'default'],
  ['認証', 'signup-error', '新規登録(エラー)', 'signup.html', 'error'],
  ['認証', 'signup-sent', '確認メールを送った', 'signup.html', 'sent'],
  ['認証', 'confirm-loading', 'メールの確認(確認中)', 'confirm.html', 'loading'],
  ['認証', 'confirm-done', 'メールの確認(完了)', 'confirm.html', 'done'],
  ['認証', 'confirm-error', 'メールの確認(期限切れ・無効)', 'confirm.html', 'error'],
  ['レビューの投稿・編集', 'review-new', 'レビュー投稿', 'review-new.html', 'default'],
  ['レビューの投稿・編集', 'review-new-filled', 'レビュー投稿(入力済み)', 'review-new.html', 'filled'],
  ['レビューの投稿・編集', 'review-new-error', 'レビュー投稿(エラー)', 'review-new.html', 'error'],
  ['レビューの投稿・編集', 'review-photo', '写真の欄の状態', 'review-photo.html', 'default'],
  ['レビューの投稿・編集', 'review-edit', 'レビュー編集', 'review-edit.html', 'default'],
  ['レビューの投稿・編集', 'review-edit-forbidden', 'レビュー編集(編集できない)', 'review-edit.html', 'forbidden'],
  ['レビューの投稿・編集', 'review-delete', '削除の確認', 'review-detail.html', 'delete'],
  ['ショップ', 'shop-detail', 'ショップ詳細', 'shop-detail.html', 'default'],
  ['ショップ', 'shop-detail-pending', 'ショップ詳細(審査待ち)', 'shop-detail.html', 'pending'],
  ['ショップ', 'shop-detail-rejected', 'ショップ詳細(却下)', 'shop-detail.html', 'rejected'],
  ['ショップ', 'shop-new', 'ショップ追加', 'shop-new.html', 'default'],
  ['ショップ', 'shop-new-error', 'ショップ追加(エラー)', 'shop-new.html', 'error'],
  // 第 2 弾
  ['プロフィール', 'profile', '自分のプロフィール', 'profile.html', 'default'],
  ['プロフィール', 'profile-other', '他のユーザーのプロフィール', 'profile.html', 'other', 'active=none'],
  ['プロフィール', 'profile-empty', '自分のプロフィール(レビューも接続もない)', 'profile.html', 'empty', null, ['ja']],
  ['プロフィール', 'profile-states', 'プロフィールの状態(コピー・接続の解除ほか)', 'profile-states.html', 'default', null, ['ja']],
  ['プロフィール', 'profile-edit', 'プロフィール編集', 'profile-edit.html', 'default', 'active=me'],
  ['プロフィール', 'profile-edit-error', 'プロフィール編集(エラー)', 'profile-edit.html', 'error', 'active=me', ['ja']],
  ['プロフィール', 'profile-delete', 'アカウント削除の確認', 'profile-edit.html', 'delete', 'active=me', ['ja']],
  ['プロフィール', 'profile-edit-forbidden', 'プロフィール編集(編集できない)', 'profile-edit.html', 'forbidden', 'active=none', ['ja']],
  ['管理', 'admin', 'ショップの管理', 'admin.html', 'default'],
  ['管理', 'admin-reject', 'ショップの管理(却下の理由)', 'admin.html', 'reject', null, ['ja']],
  ['管理', 'admin-error', 'ショップの管理(エラー)', 'admin.html', 'error', null, ['ja']],
  ['管理', 'admin-empty', 'ショップの管理(審査待ちなし)', 'admin.html', 'empty', null, ['ja']],
  ['管理', 'admin-edit', 'ショップの編集(管理)', 'admin-edit.html', 'default', null, ['ja']],
  ['管理', 'admin-edit-error', 'ショップの編集(エラー)', 'admin-edit.html', 'error', null, ['ja']],
  ['アプリの接続', 'oauth-consent', '許可の画面(読み取り)', 'oauth-consent.html', 'default', 'active=none'],
  ['アプリの接続', 'oauth-consent-write', '許可の画面(読み取りと書き込み)', 'oauth-consent.html', 'write', 'active=none'],
  ['アプリの接続', 'oauth-consent-loading', '許可の画面(確認中)', 'oauth-consent.html', 'loading', 'active=none', ['ja']],
  ['アプリの接続', 'oauth-consent-connecting', '許可の画面(接続中)', 'oauth-consent.html', 'connecting', 'active=none', ['ja']],
  ['アプリの接続', 'oauth-consent-error', '許可の画面(エラー)', 'oauth-consent.html', 'error', 'active=none', ['ja']],
  ['アプリの接続', 'oauth-signin', 'サインイン(アプリの接続の途中)', 'signin.html', 'oauth', null, ['ja']],
  ['見え方', 'visibility', '見え方の一覧', 'visibility.html', 'default', 'active=none', ['ja']],
  ['見え方', 'vis-review-other', 'レビュー詳細(他人のレビュー)', 'review-detail.html', 'other', 'user=bob', ['ja']],
  ['見え方', 'vis-review-out', 'レビュー詳細(サインインしていない)', 'review-detail.html', 'out', 'auth=out', ['ja']],
  ['見え方', 'vis-shop-out', 'ショップ詳細(サインインしていない)', 'shop-detail.html', 'out', 'auth=out', ['ja']],
  ['見え方', 'vis-shops-out', 'ショップ一覧(サインインしていない)', 'shops.html', 'out', 'auth=out', ['ja']],
  ['見え方', 'vis-shops-admin', 'ショップ一覧(管理者)', 'shops.html', 'admin', 'user=admin', ['ja']],
];
for (const lang of ['ja', 'en']) {
  for (const [group, key, title, file, state, extra, langs] of GROUPED) {
    if (langs && !langs.includes(lang)) continue;
    SCREENS.push({ key: lang === 'en' ? key + '-en' : key, title: lang === 'en' ? title + '(English)' : title, file, lang, state, extra, group });
  }
}
const SIZES = [{ key: 'pc', w: 1280, h: 800 }, { key: 'mobile', w: 375, h: 812 }];

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const result = [];
  for (const size of SIZES) {
    for (const sc of SCREENS) {
      const ctx = await browser.newContext({ viewport: { width: size.w, height: size.h }, locale: sc.lang === 'en' ? 'en-US' : 'ja-JP' });
      const page = await ctx.newPage();
      await page.goto(`file://${MOCK_DIR}/${sc.file}?lang=${sc.lang}${sc.state ? '&state=' + sc.state : ''}${sc.extra ? '&' + sc.extra : ''}`, { waitUntil: 'networkidle' });
      await page.evaluate(() => document.fonts.ready);
      const font = await page.evaluate(() => document.fonts.check('16px "Noto Sans JP"'));
      await page.waitForTimeout(300);
      const data = await page.evaluate(extract);
      const name = `${sc.key}-${size.key}`;
      fs.writeFileSync(`${OUT}/${name}.json`, JSON.stringify({ key: sc.key, title: sc.title, size: size.key, viewport: size.w, url: sc.file + (sc.state ? '?state=' + sc.state : '') + (sc.extra ? (sc.state ? '&' : '?') + sc.extra : ''), group: sc.group || null, ...data }));
      await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true });
      result.push({ name, nodes: data.nodes.length, w: data.width, h: data.height, notoLoaded: font });
      await ctx.close();
    }
  }
  // build_redesign.py が読む、画面の一覧(グループごとに Penpot のページを作る)
  fs.writeFileSync(`${OUT}/screens.json`, JSON.stringify(SCREENS.map(({ key, title, group }) => ({ key, title, group: group || null }))));
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
