// 見本の HTML の共通の処理: ヘッダーを差し込み(data-no-header の画面は除く)、バーガーの評価を組み、?lang=en なら data-en の文字に置き換える。
(() => {
  const lang = new URLSearchParams(location.search).get('lang') === 'en' ? 'en' : 'ja';
  document.documentElement.lang = lang;
  const active = document.body.dataset.active || '';
  const tab = (key, ja, en) =>
    `<a class="tab${active === key ? ' active' : ''}" href="#"><span data-en="${en}">${ja}</span>${active === key ? '<i class="uline"></i>' : ''}</a>`;
  const header = `<header class="site-header"><div class="wrap bar">
    <a class="wordmark" href="#"><span>Hamburger</span><span class="wm-pill">Evaluation</span></a>
    <nav class="tabs">${tab('shops', 'ショップ', 'Shops')}${tab('reviews', 'レビュー', 'Reviews')}<a class="tab" href="#">alice</a>${tab('', 'サインアウト', 'Sign out')}</nav>
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
