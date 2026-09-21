// Google ログインの見本(google-*.html)の共通の処理。ボタンの案(?btn=g1〜g5)・文言の案(?label=per|cont)・置き場所(?place=top|bottom)・埋め込み(?embed=1)を、
// 同じ HTML から切り替える。mock.js(ヘッダー・?lang・?state)の後に読む。
(() => {
  const q = new URLSearchParams(location.search);
  const lang = q.get('lang') === 'en' ? 'en' : 'ja';
  const state = q.get('state') || 'default';
  const G = '<svg class="glogo" viewBox="0 0 18 18" aria-hidden="true"><path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z"/><path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"/><path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z"/><path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z"/></svg>';
  const LABEL = {
    ja: { signin: 'Google でサインイン', signup: 'Google で登録', cont: 'Google で続ける', busy: 'Google に移動しています…', or: 'または' },
    en: { signin: 'Sign in with Google', signup: 'Sign up with Google', cont: 'Continue with Google', busy: 'Going to Google…', or: 'or' },
  }[lang];
  // ボタンの案。cls はボタンの class、center は、幅が文字に合う案を、フォームの中で中央に置くか
  const VARIANTS = {
    g1: { cls: 'light', name: 'G1 標準・ライト', center: true },
    g2: { cls: 'light app block', name: 'G2 ライト(このアプリの高さ・角丸)' },
    g3: { cls: 'neutral app block', name: 'G3 ニュートラル(灰色の面)' },
    g4: { cls: 'dark app block', name: 'G4 ダーク(黒い面)' },
    g5: { cls: 'light icon', name: 'G5 「G」だけ', center: true, icon: true },
  };
  // 文言の案(?label=)。per = T1 画面ごと(サインイン画面は「サインイン」、登録画面は「登録」。いまの実装)/ cont = T2 どちらも「続ける」(推奨。既定)/ mix = T3 サインインだけ「続ける」
  const textFor = (kind, labelSet) => (labelSet === 'per' ? LABEL[kind] : labelSet === 'mix' ? (kind === 'signin' ? LABEL.cont : LABEL.signup) : LABEL.cont);
  const btn = (variant, text, opts = {}) => {
    const v = VARIANTS[variant] || VARIANTS.g2;
    const logo = v.cls.includes('dark') ? `<span class="gw">${G}</span>` : G;
    // 送信中は、押せなくして(aria-disabled・フォーカスを外す)、二重に開始しない。色は薄めすぎない(opacity にしない)
    const busy = opts.busy ? ' aria-busy="true" aria-disabled="true" tabindex="-1"' : '';
    const label = opts.busy ? LABEL.busy : text;
    const style = v.center ? ' style="align-self:center"' : '';
    return v.icon
      ? `<a class="gbtn ${v.cls}" href="#" aria-label="${text}"${style}>${G}</a>`
      : `<a class="gbtn ${v.cls}" href="#"${busy}${style}>${logo}<span>${label}</span></a>`;
  };
  window.GBtn = btn;
  window.GLabel = LABEL;
  window.GVariants = VARIANTS;

  if (q.get('embed') === '1') document.body.classList.add('embed');

  // 画面の中の Google のブロック(「または」の区切り + ボタン)。body の data-gkind(signin / signup)と、data-gplace の既定(top / bottom)で決める
  const kind = document.body.dataset.gkind;
  if (kind) {
    const place = q.get('place') || document.body.dataset.gplace || 'bottom';
    const variant = q.get('btn') || 'g2';
    const text = textFor(kind, q.get('label') || 'cont');
    const button = btn(variant, text, { busy: state === 'busy' });
    const or = `<div class="gor" aria-hidden="true"><span>${LABEL.or}</span></div>`;
    const html = place === 'top' ? `<div class="gblock">${button}${or}</div>` : `<div class="gblock">${or}${button}</div>`;
    const target = document.getElementById(place === 'top' ? 'gtop' : 'gbot');
    if (target) target.innerHTML = html;
  }
  // 部品の見本(data-gbtn="signin|signup|cont" と data-v="g1"...)。比較の表で使う
  // ボタンの案・文言の案は、query(?btn=・?label=)で切り替わる(data-v があれば、それを優先。比較の表の見本)
  document.querySelectorAll('[data-gbtn]').forEach((el) => {
    const k = el.dataset.gbtn;
    const text = el.dataset.v ? LABEL[k] || LABEL.signin : textFor(k, q.get('label') || 'cont');
    el.outerHTML = btn(el.dataset.v || q.get('btn') || 'g2', text, { busy: el.dataset.busy === '1' });
  });
})();
