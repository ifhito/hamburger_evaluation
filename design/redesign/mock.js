// 見本の HTML の共通の処理: ヘッダーを差し込み(data-no-header の画面は除く)、バーガーの評価を組み、?lang=en なら data-en の文字に置き換える。
(() => {
  const params = new URLSearchParams(location.search);
  const lang = params.get('lang') === 'en' ? 'en' : 'ja';
  // ?state=xxx: data-only="a b" の要素は、state が a か b のときだけ残す(state がなければ default)。1 つの HTML から、状態違いの画面を作る
  const state = params.get('state') || 'default';
  document.querySelectorAll('[data-only]').forEach((e) => { if (!e.dataset.only.split(' ').includes(state)) e.remove(); });
  if (lang === 'en') document.querySelectorAll('.memo').forEach((e) => e.remove()); // 実装メモは、日本語の画面にだけ付ける
  document.documentElement.lang = lang;
  // ?auth=out でサインインしていない人、?user=bob でユーザー名、?active=none でどのタブも選ばない(見え方の違う画面を、同じ HTML から作る)
  const active = params.has('active') ? (params.get('active') === 'none' ? '' : params.get('active')) : (document.body.dataset.active || '');
  const viewer = params.get('user') || document.body.dataset.user || 'alice';
  const tab = (key, ja, en) =>
    `<a class="tab${active === key ? ' active' : ''}" href="#"><span data-en="${en}">${ja}</span>${active === key ? '<i class="uline"></i>' : ''}</a>`;
  const out = (params.get('auth') || document.body.dataset.auth) === 'out'; // サインインしていない人のヘッダー(サインイン・新規登録が並ぶ)
  const header = `<header class="site-header"><div class="wrap bar">
    <a class="wordmark" href="#" aria-label="BurgerStack"><svg class="wm-mark" viewBox="0 0 120 100" aria-hidden="true"><path d="M 101 88 C 101.5 89.4 97.5 90.8 90 91.5 C 82.5 92.3 71.5 92.3 60 91.5 C 48.5 90.7 36.8 88.9 28.1 86.5 C 19.5 84.1 14.1 81.1 13.7 78 C 13.3 74.9 18 71.9 26.5 69.5 C 35 67.1 47.4 65.3 60 64.5 C 72.6 63.7 85.4 63.7 94.7 64.5 C 103.9 65.2 109.4 66.6 109.6 68 L 108.7 71.9 C 108.5 72.7 108.1 72.9 107.5 72.5 L 105.8 71 C 105.2 70.6 104.2 71.1 103.3 72.5 L 100.5 76.5 C 99.6 77.8 98.2 78 96.9 76.9 L 93.2 73.6 C 91.9 72.5 90.1 72.8 88.7 74.3 L 84.1 79 C 82.7 80.5 80.6 80.5 78.9 79 L 73.9 74.5 C 72.2 73 70 73 68.2 74.5 L 62.9 79 C 61.2 80.5 58.8 80.4 57.1 78.7 L 51.8 73.4 C 50 71.7 47.8 71.4 46.1 72.7 L 41.1 76.5 C 39.4 77.8 37.3 77.5 35.9 75.7 L 31.3 70.4 C 29.9 68.7 28.1 68.1 26.8 68.9 L 23.1 71.6 C 21.8 72.4 20.4 71.9 19.5 70.4 L 16.7 65.7 C 15.8 64.1 14.8 63.3 14.2 63.5 L 12.5 64.3 C 11.9 64.5 11.5 64 11.3 62.9 L 10.4 58 C 10.6 54.9 16.1 51.9 25.3 49.5 C 34.6 47.1 47.4 45.3 60 44.5 C 72.6 43.7 85 43.7 93.5 44.5 C 102 45.2 106.7 46.6 106.3 48 C 105.9 49.4 100.5 50.8 91.9 51.5 C 83.2 52.3 71.5 52.3 60 51.5 C 48.5 50.7 37.5 48.9 30 46.5 C 22.5 44.1 18.5 41.1 19 38 C 19 22 37.4 9 60 9 C 82.6 9 101 22 101 38" fill="none" stroke="var(--logo-on-light)" stroke-width="5.5" stroke-linecap="round" stroke-linejoin="round"/></svg><span class="wm-t"><span class="wm-a">Burger</span><span class="wm-b">Stack</span></span></a>
    <nav class="tabs">${tab('shops', 'ショップ', 'Shops')}${tab('reviews', 'レビュー', 'Reviews')}${out ? tab('signin', 'サインイン', 'Sign in') + tab('signup', '新規登録', 'Sign up') : tab('me', viewer, viewer) + tab('__signout', 'サインアウト', 'Sign out')}</nav>
    <div class="lang"><button class="lang-btn${lang === 'ja' ? ' on' : ''}">JA</button><button class="lang-btn${lang === 'en' ? ' on' : ''}">EN</button></div>
  </div></header><div class="rule"></div>`;
  if (!document.body.hasAttribute('data-no-header')) document.body.insertAdjacentHTML('afterbegin', header);
  // ロゴの記号(黄色の 1 本の線)を、Penpot のパス 1 つとして写せるように、図形の一覧を付ける
  document.querySelectorAll('svg.wm-mark').forEach((el) => {
    el.__icon = { name: 'BurgerStack のロゴ(記号)', vb: [120, 100], shapes: [{ part: 'ロゴの記号', d: el.querySelector('path').getAttribute('d'), fill: null, stroke: getComputedStyle(document.documentElement).getPropertyValue('--logo-on-light').trim(), sw: 5.5, bbox: [10.44, 9.0, 99.12, 83.1], round: true }] };
  });

  // バーガーの評価: data-score(数字に出す値)・data-size(lg / md / sm / xs)から、線画のバーガーを水位のように塗る(rating-icons.js)。data-decor があれば、飾りとして読み上げない
  document.querySelectorAll('.burger').forEach((el) => window.Burger.mount(el, lang));

  if (lang === 'en') {
    document.querySelectorAll('[data-en]').forEach((e) => { e.textContent = e.dataset.en; });
    document.querySelectorAll('[data-en-ph]').forEach((e) => { e.placeholder = e.dataset.enPh; });
  }
})();
