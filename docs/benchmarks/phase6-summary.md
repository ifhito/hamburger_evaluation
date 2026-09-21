# Phase 6: フロントエンド 4 候補のまとめ

- 計測日: 2026-09-22
- 計測元: 日本(自宅回線)から
- 同じビルド成果物(`frontend/dist`、496KB)を 4 社に配置
- 転送先の API は Cloud Run(東京)。Phase 4 で唯一全機能が動いたホスト

## 結果

| ホスト | HTML p50 | `/api` 転送 p50 | deep link | 転送設定 | 備考 |
|---|---|---|---|---|---|
| **Cloudflare Workers** | 54ms | **285ms** | ○ | JS 11 行 | 転送が最速 |
| **Render Static** | **40ms** | 505ms | ○ | YAML 7 行 | HTML が最速 |
| Netlify | 287ms | 827ms | ○ | TOML 9 行 | 転送が最も遅い |
| Vercel | 44ms | **計測不能** | ○ | JSON 10 行 | **Bot 対策でブロックされた** |

参考: API 直叩き(Cloud Run)は p50 **288ms**。

### ばらつき

| ホスト | HTML(p95 / 最大) | 転送(p95 / 最大) |
|---|---|---|
| Cloudflare | 84ms / 92ms | 394ms / 556ms |
| Render | **49ms / 49ms** | 776ms / 1,031ms |
| Netlify | 963ms / 1,519ms | 1,318ms / 1,389ms |
| Vercel | 88ms / 224ms | — |

**Render の HTML 配信が最も安定している**(p50 40ms、最大 49ms)。
Netlify は p50 が 287ms なのに最大 1,519ms と大きく揺れる。

## 分かったこと

### 1. HTML の速さと転送の速さは連動しない

| ホスト | HTML | 転送 | 順位の逆転 |
|---|---|---|---|
| Render | **40ms**(1 位) | 505ms(3 位) | ○ |
| Cloudflare | 54ms(3 位) | **285ms**(1 位) | ○ |

**静的配信の速さだけで選ぶと、API 経由で損をする。** 同じ事業者でも、
自前のファイルを返す経路と、外部へ転送する経路で性能が違う。

### 2. Netlify の転送が突出して遅い

転送の上乗せ(転送 p50 − 直叩き p50)を比べると差が明確になる。

| ホスト | 上乗せ |
|---|---|
| Cloudflare | **−3ms**(下記の注記を参照) |
| Render | +217ms |
| Netlify | **+539ms** |

Netlify は Cloudflare より 542ms も遅い。同じことをしているのに、
転送先への経路が大きく違うとしか説明できない。

### 注記: 「上乗せ 0ms」は計測条件の差である

Cloudflare 経由の最小値 131ms が、直叩きの最小値 266ms より**速い**という
矛盾が出た。物理的にありえないため調べたところ、キャッシュではなかった
(`cf-cache-status: DYNAMIC`、`x-cloud-trace-context` あり = 毎回 Cloud Run に到達)。

原因は計測方法にある。`latency.sh` は毎回新しい接続を張るため、
**直叩きは毎回 TLS の確立を含む**。一方 Cloudflare 経由では、
**Worker と Cloud Run の間の接続が使い回される**。条件が揃っていない。

**「転送の上乗せがゼロ」ではなく「Cloudflare が接続を再利用するぶん、
新規接続の直叩きと同等になった」が正確である。** ブラウザは接続を再利用するため、
実際の利用では直叩きのほうが速い。

### 3. 転送設定の記述量に大きな差はない

| ホスト | 方式 | 実効行数 |
|---|---|---|
| Render | YAML の宣言 | 7 |
| Netlify | TOML の宣言 | 9 |
| Vercel | JSON の宣言 | 10 |
| Cloudflare | **JavaScript のコード** | 11 |

[ADR-0001](../adr/0001-frontend-deploy-target.md) では「Cloudflare は設定ではなく
コードを書く手間がある」と減点していたが、**行数の差は 4 行しかない**。
しかも転送は最速だった。**減点の根拠は薄かった。**

### 4. SPA の書き換えと API 転送は「順序」が命

4 社すべてで、`/api/*` の転送を `/*` → `index.html` より**先に**評価させる必要がある。
逆にすると、すべてが `index.html` に吸われて API へ届かない。

Vercel だけは正規表現で除外する書き方になった。

```json
{ "source": "/((?!assets/).*)", "destination": "/index.html" }
```

## ハマった点

### Render: Rewrite と Redirect を間違えると壊れる

最初の設定では Action が Redirect になっており、`/api/meta` が
`301 Moved Permanently` で Cloud Run の URL を返した。

ブラウザが直接 API に飛ばされるため**オリジンが変わり、CORS で失敗する**。
Go 側に CORS の実装がないので、アプリは動かない。

**Rewrite はサーバー側で裏に取りに行き URL が変わらない。Redirect はブラウザに
別の場所を指示する。** 同一オリジンを保つには Rewrite が必須である。

### Vercel: 既定で公開されていない

デプロイ直後、すべてのパスが 302 で `vercel.com/sso-api` にリダイレクトされた。
**Deployment Protection(Vercel Authentication)が既定で有効**になっており、
ログインしないと閲覧できない。

ダッシュボードで明示的に無効化する必要がある。**料金表からは読み取れない制約**である。

### Vercel: 計測でブロックされた

50 回のリクエストを送ったところ、以後すべてが 403 になった。

```text
x-vercel-mitigated: challenge
x-vercel-challenge-token: ...
```

Bot 対策(Attack Challenge)が発動している。User-Agent をブラウザ相当に変えても
解除されず、**IP 単位で遮断**されていると考えられる。

他の 3 社では同じ 50 回の計測が問題なく通った。**Vercel だけが厳しい。**

実用上の懸念もある。SPA は 1 画面で複数の API を叩くため、
**熱心な利用者や、共有 IP(オフィス・学校)からの複数人の利用で誤検知されうる**。

### Cloudflare: CLI のログインが非対話環境で失敗する

`wrangler login` は端末との対話が前提で、このセッションのシェルでは
`isInteractive: false` と判定されて 71 秒でタイムアウトした。

一方 **Netlify と Vercel の CLI はバックグラウンドでもブラウザ認証の待機ができた**。
CLI の設計思想の違いである。

## CLI でのデプロイ可否

| ホスト | CLI | 認証 | デプロイ |
|---|---|---|---|
| Cloudflare | `wrangler`(インストール済み) | **対話必須** | `wrangler deploy` |
| Netlify | `npx netlify-cli` | バックグラウンドで可 | `deploy --prod --dir=dist` |
| Vercel | `npx vercel` | バックグラウンドで可 | `deploy --prod --yes` |
| Render | なし(Git 連携) | — | ダッシュボードで Blueprint |

**3 社は CLI で完結する。** Render だけはサービス作成がダッシュボード操作になる。

## ADR への反映

[ADR-0001](../adr/0001-frontend-deploy-target.md) の決定は
「API と同じ事業者に置く」だった。**Phase 4 で API が Cloud Run になったため、
この前提は失われた**(Cloud Run に静的ホスティングを併設しない)。

実測を踏まえた評価。

| ホスト | 評価 |
|---|---|
| **Cloudflare Workers** | **転送が最速。帯域無制限・商用可。コードを書く手間は 4 行分でしかない** |
| Render Static | HTML は最速で最も安定。転送は中位。Rewrite の設定に注意 |
| Netlify | 転送が最も遅い。設定は簡単だが積極的に選ぶ理由が薄い |
| Vercel | **非商用限定に加え、既定で非公開、さらに Bot 対策でブロックされる**。制約が 3 つ |

**第一候補を Cloudflare Workers に変えるべきである。**

## 未計測

- **ブラウザ経由の写真アップロード**。フロント経由で実際に通るか
- **OAuth の `redirect_uri` 登録**。プレビュー環境が使えないことの実証
- Vercel の転送レイテンシ(ブロック解除後)
- 接続を再利用した条件での再計測(上の注記を参照)
