// デザイン専用。API呼び出し・認証・投稿・永続化は行わない。
const page = document.querySelector('#page');
const guest = new URLSearchParams(location.search).get('guest') === '1';
const notice = document.querySelector('#notice');
function explain(message) { document.querySelector('#notice-text').textContent = message; notice.showModal(); }
const cards = () => `<div class="cards">${['クラシックバーガー','ダブルチーズバーガー','アボカドバーガー'].map((name,i) => `<article class="card"><img src="../refresh/assets/sample-burger.jpg" alt="ハンバーガーのサンプル写真"><div class="card-body"><p class="meta">サンプルのお店 ${i+1}</p><h3>${name}</h3><div class="rating"><span class="burger" data-score="${4+i%2}" data-size="sm" data-variant="A"></span><strong>${4+i%2}</strong></div><p>また食べに行きたくなる、お気に入りの一口。</p><p class="meta" style="margin-top:20px">食べた日：2026年9月20日</p></div></article>`).join('')}</div><p class="sample">デザイン確認用のサンプルです。写真と店舗・商品名は対応していません。</p>`;
const titles = {shops:'お店を探す',burgers:'バーガーを探す',reviews:'みんなのレビュー'};
function searchContent(route, record = false) { return `<div class="intro"><p class="eyebrow">${record?'食べた記録':'見つける'}</p><h1>${record?'どのお店で食べましたか？':titles[route]}</h1><p class="muted">${record?'お店を選んで、今日の一口を記録しましょう。':'次に食べたい一口を探そう。'}</p></div>${record?'':`<nav class="search-tabs" aria-label="探す対象">${Object.entries(titles).map(([k,v])=>`<a href="#${k}" ${k===route?'aria-current="page"':''}>${k==='shops'?'お店':k==='burgers'?'バーガー':'レビュー'}</a>`).join('')}</nav>`}<form class="toolbar"><select aria-label="並び替え"><option>新しい順</option><option>評価の高い順</option></select><input aria-label="検索キーワード" placeholder="${record?'食べたお店の名前':route==='shops'?'お店の名前':route==='burgers'?'バーガーの名前':'キーワード'}で探す"><button class="button" type="submit">検索</button></form>${cards()}`; }
function render() {
 const route = location.hash.slice(1) || 'home';
 if (route === 'main') return;
 const section = ['shops','burgers','reviews'].includes(route)?'discover':route;
 document.querySelectorAll('[data-nav]').forEach(a=>{a.removeAttribute('aria-current');if(a.dataset.nav===section)a.setAttribute('aria-current','page');});
 document.querySelectorAll('.desktop-links a,.about-link,.profile-button,.record-button').forEach(a=>{a.removeAttribute('aria-current');if(a.getAttribute('href')==='#'+route || (a.getAttribute('href')==='#discover' && ['shops','burgers','reviews'].includes(route)))a.setAttribute('aria-current','page');});
 if (route==='discover') page.innerHTML=searchContent('shops');
 else if (route==='home') page.innerHTML=`<section class="hero"><p class="eyebrow">BURGER STACK</p><h1>次の一口に、<br>出会おう。</h1><p>探して、食べて、記録する。<br>あなたの好きが、誰かのきっかけに。</p><a class="button" href="#discover">次の一口を探す <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M 4 12 L 20 12 M 14 6 L 20 12 L 14 18"/></svg></a></section><section><div class="section-heading"><h2>みんなの食べた記録</h2><a href="#reviews">すべて見る <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M 4 12 L 20 12 M 14 6 L 20 12 L 14 18"/></svg></a></div>${cards()}</section>`;
 else if(titles[route]) page.innerHTML=searchContent(route);
 else if(route==='discover') page.innerHTML=searchContent('shops');
 else if((route==='record'||route==='profile')&&guest) page.innerHTML=`<section class="empty"><h1>${route==='record'?'食べた一口を、記録しよう。':'自分だけのバーガーの記録を。'}</h1><p>サインインすると、食べたバーガーを記録して振り返れます。</p><button class="button" data-explain="サインイン後は${route==='record'?'お店を選ぶ画面':'自分のプロフィール'}に戻ります。このプレビューでは認証しません。">サインイン・新規登録</button></section>`;
 else if(route==='record') page.innerHTML=searchContent('shops',true);
 else if(route==='profile') page.innerHTML=`<div class="profile-hero"><span class="avatar"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M 16 8 C 16 10.21 14.21 12 12 12 C 9.79 12 8 10.21 8 8 C 8 5.79 9.79 4 12 4 C 14.21 4 16 5.79 16 8 M 4 21 L 4 19 C 4 8.33 20 8.33 20 19 L 20 21"/></svg></span><div><p class="eyebrow">マイページ</p><h1>バーガー好きさん</h1></div></div><div class="profile-links"><button data-explain="プロフィール編集へ進みます。">プロフィールを編集</button><button data-explain="プロフィール内の接続済みアプリへ進みます。">接続済みアプリ</button><button data-explain="サインアウトを確認し、完了後はホームへ戻ります。">サインアウト</button></div><div class="section-heading"><h2>食べた記録</h2></div>${cards()}`;
 else if(route==='about') page.innerHTML=`<section class="hero"><p class="eyebrow">Burger Stackについて</p><h1>世の中に、もっと<br>ハンバーガーを！！！</h1><p>お気に入りの一口を探して、<br>ハンバーガーの記録を積み重ねよう。</p></section><section class="empty"><h2>好きな一口を、分かち合う。</h2><p>紹介ページへの入口は、どの画面でもヘッダーに。ここからサービスの思いや使い方に出会えます。</p><a class="button" href="#discover">次の一口を探す</a></section>`;
 else if(route==='ai') page.innerHTML=`<section class="empty"><p class="eyebrow">AIから使う</p><h1>いつものAIと、<br>Burger Stackを。</h1><p>ここから既存のMCP接続案内ページへ進みます。アプリ実装時のリンク先は /mcp です。</p><a class="button" href="#home">ホームへ</a></section>`;
 else {location.hash='home';return;}
 document.querySelectorAll('.burger').forEach(el=>window.Burger.mount(el,'ja'));
 document.querySelectorAll('[data-explain]').forEach(button=>button.addEventListener('click',()=>explain(button.dataset.explain)));
 document.querySelector('.toolbar')?.addEventListener('submit',event=>{event.preventDefault();explain('検索の配置と導線のプレビューです。実装時は検索条件をAPIへ送り、結果を表示します。');});
 document.title = `${(route==='discover'?'次の一口を探す':titles[route]) || ({home:'ホーム',record:'記録する',profile:'マイページ',about:'Burger Stackについて',ai:'AIから使う'}[route])} — Burger Stack デザイン案`;
}
document.querySelector('#language').addEventListener('click',()=>explain('言語切り替えの配置案です。実装時は既存の日本語・英語設定に接続します。'));
window.addEventListener('hashchange',()=>{render();window.scrollTo(0,0);document.querySelector('main').focus({preventScroll:true});});
render();
