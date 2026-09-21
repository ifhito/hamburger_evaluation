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
    <a class="wordmark" href="#"><span>Hamburger</span><span class="wm-pill">Evaluation</span></a>
    <nav class="tabs">${tab('shops', 'ショップ', 'Shops')}${tab('reviews', 'レビュー', 'Reviews')}${out ? tab('signin', 'サインイン', 'Sign in') + tab('signup', '新規登録', 'Sign up') : tab('me', viewer, viewer) + tab('__signout', 'サインアウト', 'Sign out')}</nav>
    <div class="lang"><button class="lang-btn${lang === 'ja' ? ' on' : ''}">JA</button><button class="lang-btn${lang === 'en' ? ' on' : ''}">EN</button></div>
  </div></header><div class="rule"></div>`;
  if (!document.body.hasAttribute('data-no-header')) document.body.insertAdjacentHTML('afterbegin', header);

  // バーガーの評価: data-score(数字に出す値)・data-size(lg / md / sm / xs)から、線画のバーガーを水位のように塗る(rating-icons.js)。data-decor があれば、飾りとして読み上げない
  document.querySelectorAll('.burger').forEach((el) => window.Burger.mount(el, lang));

  if (lang === 'en') {
    document.querySelectorAll('[data-en]').forEach((e) => { e.textContent = e.dataset.en; });
    document.querySelectorAll('[data-en-ph]').forEach((e) => { e.placeholder = e.dataset.enPh; });
  }
})();
