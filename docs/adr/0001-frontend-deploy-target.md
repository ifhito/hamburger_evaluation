# ADR-0001: フロントエンドのデプロイ先

- ステータス: 承認(Accepted)。2026-09-24 に本番で採用
- 日付: 2026-09-21(2026-09-20 初版を、Rails 退役と OAuth 認可サーバーの追加を受けて改訂)
- 対象: `frontend/`(React 19 + Vite の静的 SPA)
- 方針: **無料で始める**。読者・運用者は個人開発者を想定する
- 関連: [ADR-0002](0002-backend-deploy-target.md)(API)、[ADR-0004](0004-photo-storage.md)(写真)

## 背景

フロントエンドは `pnpm build` で `dist/` を出す静的 SPA である。SSR も Node ランタイムも不要。

### API への接続方法は 2 通りある

`frontend/src/api/client/buildApiClient.ts:18` は次のとおり。

```ts
axios.create({ baseURL: import.meta.env.VITE_API_BASE_URL ?? "/api" })
```

- **A. 同一オリジン方式**(既定): ホスト側で `/api/*` を API へリライトする。CORS 不要。
- **B. 直接接続方式**: `VITE_API_BASE_URL` に API の公開 URL を入れる。**Go 側に CORS ミドルウェアが必要**。

現時点で `backend-go` に CORS の実装はない(`internal/adapter/handler` に `Access-Control-*` の記述なし)。
B を採るなら実装を足す作業が先に発生する。したがって**当面は A を前提に選ぶ**。

### 写真の配信経路は保存先で変わる

- `PHOTO_STORAGE=disk`: 写真は API の `GET /photos/*` が配信する。フロント側に `/photos/*` のリライトも要る
  (`frontend/nginx.conf:22` とローカルの Vite proxy が既にそうしている)。
- `PHOTO_STORAGE=s3`: `GET /photos/*` は登録されず、写真の URL はバケットの公開ドメインを指す。リライト不要。

[ADR-0004](0004-photo-storage.md) で S3 を選ぶので、**本番ではリライトは `/api/*` の 1 本で足りる**。

### OAuth の窓口はフロントを通らない

OAuth 2.1 の認可サーバーが `backend-go` に実装された。窓口は次のとおりで、**いずれも `/api` 配下ではない**。

```text
GET  /.well-known/oauth-authorization-server
GET  /oauth/authorize
POST /oauth/token
POST /oauth/revoke
```

`OAUTH_ISSUER` は API 自身の公開 URL である。つまり **API はフロントの裏に隠さず、独自の公開 URL を持つ**。
フロント側でこれらを中継する必要はない。

ただし逆向きの依存が 1 つできた。`OAUTH_CONSENT_URL` の既定は `<APP_BASE_URL>/oauth/authorize` で、
**フロントの画面を指す**。フロントの URL が API の設定に入るため、デプロイの順序に影響する。
なお同意画面は `frontend/src` にまだ実装されていない(2026-09-21 時点)。

### プレビュー環境が OAuth と両立しない

OAuth の戻り先(`redirect_uri`)は完全一致で事前登録する。https またはループバック http のみ。
PR ごとに URL が変わるプレビュー環境は**事前登録できない**ため、プレビューでは OAuth が動かない。
初版では「PR プレビュー」を評価軸に入れていたが、**重みを下げる**。

## 評価軸

1. **無料枠の条件**(期限、帯域上限、商用利用の可否、カード登録の要否)
2. `/api/*` を外部 URL へリライトできるか
3. 独自ドメイン・HTTPS の手間
4. 設定量・運用の手間
5. API と同居できるか(環境変数とデプロイを 1 か所にまとめられるか)
6. ~~PR プレビュー環境~~ → OAuth が動かないため参考情報に格下げ

## 無料で始められる候補

条件は 2026-09 時点の目安。契約前に公式で再確認すること。

| 候補 | 無料枠の条件 | `/api` リライト | ドメイン/HTTPS | 設定量 | API と同居 |
|---|---|---|---|---|---|
| Render Static Site | 帯域 100GB/月。無期限。カード不要 | ◎ `render.yaml` の rewrites | ◎ 自動 | 小 | ◎ Go API と同じダッシュボード |
| Cloudflare Workers(Static Assets) | 帯域無制限。商用可。カード不要 | ○ Worker のコードで `fetch` 転送 | ◎ | 中 | ✕ |
| Netlify | 帯域 100GB/月。商用可。カード不要 | ◎ `_redirects` に 1 行 | ◎ | 最小 | ✕ |
| Vercel | 帯域 100GB/月。**非商用限定**。カード不要 | ◎ `vercel.json` の rewrites | ◎ 自動 | 最小 | ✕ |
| Koyeb(Static) | 無期限(条件は要確認)。カード要 | ○ | ◎ | 小 | ◎ 東京で API も同居 |
| Zeabur | 無料枠あり(条件は要確認) | ○ | ◎ | 小 | ◎ 東京 |

### 所感

- **Render**: フロント・Go API・(必要なら)Postgres を 1 か所にまとめられる。`APP_BASE_URL` と
  `OAUTH_CONSENT_URL` の突き合わせも同じ画面で済む。無料枠は無期限でカード不要。
- **Cloudflare Workers**: 帯域無制限・商用可という条件は唯一。リライトは設定でなくコードを書く。
- **Netlify**: 記述量は最小。Vercel を選ばない理由がある場合の第一代替。
- **Vercel**: 体験は最良だが Hobby が非商用限定。収益化した時点で月 20 ドル。
- **Koyeb / Zeabur**: 東京で API と同居できる。事業者の規模は主要候補より小さい。
  Koyeb は 2026-09-21 時点で利用登録ができず、今回は候補から外している([ADR-0002](0002-backend-deploy-target.md))。

## それ以外(有料のみ、または無料枠が期限付き)

| 候補 | 最小コスト | 備考 |
|---|---|---|
| AWS S3 + CloudFront / Amplify | 12 か月無料の後は従量 | 設定量が多い。API が AWS のときのみ |
| Bunny.net | 月 1 ドル程度 | 東京 PoP。デプロイは手動 |
| DigitalOcean App Platform | 静的 3 つまで無料(帯域 1GB/月) | 帯域が小さすぎる |
| GitHub Pages / Firebase Hosting | 無料 | リライト不可。B 方式(CORS 実装)が前提になる |

## 実測(2026-09-22、Phase 6)

同じビルド成果物を 4 社に配置し、転送先を Cloud Run(東京)にそろえて計測した。

| ホスト | HTML p50 | `/api` 転送 p50 | deep link | 転送設定 |
|---|---|---|---|---|
| **Cloudflare Workers** | 54ms | **285ms** | ○ | JS 11 行 |
| Render Static | **40ms** | 505ms | ○ | YAML 7 行 |
| Netlify | 287ms | 827ms | ○ | TOML 9 行 |
| Vercel | 44ms | **計測不能** | ○ | JSON 10 行 |

参考: API 直叩きは 288ms。

### 判明した点

- **HTML の速さと転送の速さは連動しない。** Render は HTML が最速(40ms)だが転送は 3 位、
  Cloudflare は HTML が 3 位だが転送は最速。**静的配信の速さだけで選ぶと API 経由で損をする**
- **記述量の差は小さい。** 本 ADR では「Cloudflare はコードを書く手間がある」と減点していたが、
  実際の差は 4 行しかなく、しかも転送は最速だった。**減点の根拠は薄かった**
- **Vercel の制約が 3 つに増えた。** 非商用限定に加え、**既定で非公開**(Deployment Protection)、
  さらに **50 回の計測で Bot 対策に遮断された**(`x-vercel-mitigated: challenge`)。
  他の 3 社では同じ計測が通った
- **Render は Rewrite と Redirect の取り違えで壊れる。** Redirect だとブラウザが直接 API に
  飛ばされ、オリジンが変わって CORS で失敗する

### 前提が変わった

本 ADR は「API と同じ事業者に置く」ことを決定の根拠にしていたが、
[ADR-0002](0002-backend-deploy-target.md) で API が Cloud Run になったため、
**同居という選択肢自体が消えた**。4 社とも「API は別事業者」という同じ条件になり、
比較としてはむしろ公平になった。

### 決定を変える

**第一候補を Cloudflare Workers にする。** 転送が最速で、帯域無制限・商用可という
条件も唯一である。コードを書く手間は 4 行分でしかなかった。

Vercel は制約が 3 つあり、候補から外す。

詳細は `docs/benchmarks/phase6-summary.md` にある。

## 決定

**Cloudflare Workers(Static Assets)** に置き、独自ドメイン **`burger-stack.com`** で公開する。`/api/*` は Worker のコード(`frontend/worker/index.js`)が Cloud Run の API へ転送する。

検証前の案は「API と同じ事業者に置く」だった。API が Cloud Run に決まり([ADR-0002](0002-backend-deploy-target.md))、Cloud Run には静的ホスティングがないので、同居の前提がなくなった。そのうえで 4 候補を実測し、次の理由で Cloudflare を選んだ。

- `/api` への転送が 4 候補で最も速かった(p50 285ms。Render 505ms、Netlify 827ms)
- 帯域が無制限で、商用利用もできる。Vercel の Hobby は非商用に限られる
- 設定ではなくコードを書く手間は、行数で 4 行の差しかなかった

## 結果・影響

- **Worker に 2 つの設定が要る。** どちらかが欠けると、`fetch` で呼ぶ API は動くのに、Google ログインのようなページ遷移だけが 404 になる。
  - `wrangler.toml` の `run_worker_first = ["/api/*"]`。ページ遷移ではアセット配信が Worker より先に働き、`index.html` を返してしまうため
  - `worker/index.js` の `fetch(..., { redirect: "manual" })`。既定では Worker が 302 を自分で追いかけるため
- **独自ドメインはダッシュボードで付ける。** CD は版の入れ替えしかしないので、ドメインの設定に触れない。
- **版のプレビュー URL を CD が使う。** 新しい版をプレビュー URL で確かめてから切り替える。プレビュー URL は workers.dev の上にしか作れないので、`preview_urls = true` を明示している。公開の入口としての workers.dev は止めてよい。
- 許可を尋ねる画面(`/oauth/authorize`)は frontend に実装済み。
- **PR プレビューでは OAuth が動かない**(`redirect_uri` を事前登録できない)。必要になったら常設のステージングを 1 つ作る。
- 写真は R2 から直接配信するので、`/photos/*` の転送は本番では不要。ローカルの Vite proxy と `frontend/nginx.conf` は disk 運用のために残す。
- 配り方と確かめ方は [本番へのデプロイ](../production-deploy.md) にある。
