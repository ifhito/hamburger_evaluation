// 見本の HTML の共通の処理: ヘッダーを差し込み、?lang=en なら data-en の文字に置き換える。
(() => {
  const lang = new URLSearchParams(location.search).get('lang') === 'en' ? 'en' : 'ja';
  document.documentElement.lang = lang;
  const active = document.body.dataset.active || '';
  const tab = (key, ja, en) =>
    `<a class="tab${active === key ? ' active' : ''}" href="#"><span data-en="${en}">${ja}</span>${active === key ? '<i class="uline"></i>' : ''}</a>`;
  const header = `<header class="site-header"><div class="wrap bar">
    <a class="wordmark" href="#"><span>Hamburger</span> <span class="wm-accent">Evaluation</span></a>
    <nav class="tabs">${tab('shops', 'ショップ', 'Shops')}${tab('reviews', 'レビュー', 'Reviews')}<a class="tab" href="#">alice</a>${tab('', 'サインアウト', 'Sign out')}</nav>
    <div class="lang"><button class="lang-btn${lang === 'ja' ? ' on' : ''}">JA</button><button class="lang-btn${lang === 'en' ? ' on' : ''}">EN</button></div>
  </div></header><div class="rule"></div>`;
  document.body.insertAdjacentHTML('afterbegin', header);
  if (lang === 'en') {
    document.querySelectorAll('[data-en]').forEach((e) => { e.textContent = e.dataset.en; });
    document.querySelectorAll('[data-en-ph]').forEach((e) => { e.placeholder = e.dataset.enPh; });
  }
})();
