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

  // バーガーの評価: data-score(0〜5)を、いちばん近い 0.5 に丸めて、上の段から並べる。data-decor があれば、飾りとして読み上げない
  document.querySelectorAll('.burger').forEach((el) => {
    const score = Math.round(parseFloat(el.dataset.score || '0') * 2) / 2;
    el.innerHTML = [5, 4, 3, 2, 1].map((n) => {
      const full = score >= n;
      const half = !full && score >= n - 0.5;
      return `<i class="l l${n}${full ? ' on' : ''}">${half ? '<b class="hf"></b>' : ''}</i>`;
    }).join('');
    if (el.hasAttribute('data-decor')) el.setAttribute('aria-hidden', 'true');
    else { el.setAttribute('role', 'img'); el.setAttribute('aria-label', lang === 'en' ? `${el.dataset.score} out of 5` : `5 段階中 ${el.dataset.score}`); }
  });

  if (lang === 'en') {
    document.querySelectorAll('[data-en]').forEach((e) => { e.textContent = e.dataset.en; });
    document.querySelectorAll('[data-en-ph]').forEach((e) => { e.placeholder = e.dataset.enPh; });
  }
})();
