# ADR-0001: フロントエンドのデプロイ先

- ステータス: 提案中(Proposed)
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

## 決定(案)

API(ADR-0002)と同じ事業者に置く。

- API が **Render** → フロントも **Render Static Site**
- API が **Zeabur**(東京優先) → フロントも同じ事業者の静的ホスティング
- API が **Cloud Run** → フロントは **Cloudflare Workers**(帯域無制限)

同居を優先するのは、`APP_BASE_URL` と `OAUTH_CONSENT_URL` と `OAUTH_ISSUER` という
**環境をまたいで整合させる URL が 3 つある**ためで、別事業者にすると設定ミスの余地が増える。

Vercel は非商用限定の条項があるため第一候補にしない。

## 結果・影響

- 追加するもの: `/api/*` のリライト定義 1 ファイル。`buildApiClient.ts` は変更しない。
- **同意画面の実装が必要**。`OAUTH_CONSENT_URL` が指す `/oauth/authorize` のページがフロントに存在しない。
  デプロイ検証より先か、少なくとも OAuth の疎通確認より先に要る。
- **PR プレビューでは OAuth が動かない**。プレビューを使う場合、OAuth を伴わない画面に限る。
  常設のステージング環境を 1 つ作り、その URL を `redirect_uri` に登録するほうが現実的。
- 写真を S3 にするため、`/photos/*` のリライトは本番では不要。ローカルの Vite proxy と
  `frontend/nginx.conf` は disk 運用のために残す。
- B 方式(直接接続)に切り替える場合は、Go に CORS ミドルウェアを実装してから。本 ADR を更新すること。
