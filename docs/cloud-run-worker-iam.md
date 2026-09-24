# Workers から IAM 認証付きで Cloud Run を呼ぶ

Web アプリと MCP は既存の Cloudflare Worker を通し、Cloud Run は専用サービスアカウントの ID トークンを確認する。
利用者の JWT / OAuth トークンは `Authorization` のまま保持し、インフラ認証は `X-Serverless-Authorization` に分ける。
これは Cloud Run の直 URL への匿名呼び出しを拒否する構成である。公開 Worker のレート制限・DDoS 対策を代替するものではない。

## 変更する値

| 項目 | 値 |
| --- | --- |
| GCP project | `charged-chain-297113` |
| Cloud Run service / region | `burger-stack` / `asia-northeast1` |
| Worker | `hamburger-frontend` |
| Worker `API_ORIGIN` | `https://burger-stack-421794940461.asia-northeast1.run.app` |
| Worker / CI `CLOUD_RUN_AUDIENCE` | `https://burger-stack-azcarxd7mq-an.a.run.app`（Cloud Run の `status.url`） |
| Worker Secret | `GCP_SERVICE_ACCOUNT_JSON` |
| Cloud Run `OAUTH_ISSUER` | `https://burger-stack.com/api` |
| Cloud Run `OAUTH_RESOURCE_URL` | `https://burger-stack.com/api/mcp` |
| Cloud Run `OAUTH_CONSENT_URL` | `https://burger-stack.com/oauth/authorize` |
| MCP の新しい接続先 | `https://burger-stack.com/api/mcp` |

`API_ORIGIN` と `CLOUD_RUN_AUDIENCE` は Worker の設定値。秘密鍵を `wrangler.toml`、GitHub Variables、フロントエンドのビルド変数へ入れない。
Google ログインの callback は既存の `https://burger-stack.com/api/auth/google/callback` を維持する。
`MCP_ALLOWED_ORIGINS` には利用中の正当な Origin を残す。公開URLの移行に合わせて `https://burger-stack.com` が許可されていることを確認する。

## 導入順序

1. Worker 専用サービスアカウント `cloudflare-api-invoker` を作成する。対象 Cloud Run サービスにだけ `roles/run.invoker` を付与する。プロジェクト全体の管理権限は付けない。
2. デプロイ確認用の `github-deployer@charged-chain-297113.iam.gserviceaccount.com` にも対象サービスの `roles/run.invoker` を付与する。既存の GitHub WIF で ID トークンを発行できることを確認する。
3. Worker 専用アカウントの JSON 鍵を Workers Secrets の `GCP_SERVICE_ACCOUNT_JSON` に登録する。鍵はチャット・ログ・リポジトリへ出さない。組織ポリシーが鍵の作成を禁止している場合は、禁止を解除せず認証連携方式を再検討する。
4. IAM認証を要求する隔離した Cloud Run 検証先で、Worker の署名・トークン交換・転送を確認する。公開状態の本番へ200で届くだけでは、IAM認証に成功した証拠にならない。
5. この変更を含む Worker を候補版へ配り、ログイン・店舗一覧・写真投稿を確認する。未設定やトークン交換失敗の場合、API は503を返し、匿名転送へ切り替えない。
6. CI の API 確認が ID トークンを送れる版になったことを確認する。候補リビジョンの URL へ送る場合もトークンの宛先はサービスの `status.url` にする。
7. Cloud Run の OAuth 設定を上表の公開 URL に更新し、Worker を本番へ切り替える。MCP クライアントを新しい URL で接続し直し、再認可・読み取り・書き込みを確認する。旧発行元・宛先のトークンがそのまま使えるとは想定しない。
8. 接続確認後、Cloud Run の「認証が必要 → IAM」を有効にする。サービスと上位の IAM に `allUsers` / `allAuthenticatedUsers` などの広い呼び出し許可がないことも確認する。**IAM チェック無効のままでは、権限表から匿名権限を消しても保護されない。**
9. 認証なしの Cloud Run `/up` が拒否され、Worker `/api/up`・Web ログイン・MCP が動くことを確認する。CI の候補版・本番の確認も通す。

Cloudflare は Google Cloud の外部にあるため、ここでは ingress を `all` のまま IAM で呼び出しを制限する。`internal` へ変更すると Worker からも届かなくなる。

## 公開する OAuth の発見用 URL

- `/.well-known/oauth-authorization-server/api` → API の `/.well-known/oauth-authorization-server`
- `/.well-known/oauth-protected-resource/api/mcp` → API の同じパス
- path を省略するクライアント向けに、上記それぞれの root の発見用 URL も転送する。

これらは SPA の HTML を返してはいけない。`wrangler.toml` の `run_worker_first` と Worker の転送ルールで扱う。
OAuth の許可画面 `/oauth/authorize` は Web アプリの画面なので、そのまま静的配信に任せる。

## 鍵の運用と切り戻し

鍵の担当者と更新周期を決める。新しい鍵を Secret に反映して接続確認した後、古い鍵を無効化・削除する。
Worker は Google ID トークンを有効期限の1分前まで isolate 内で再利用し、同時のトークン取得をまとめる。
鍵や宛先が変わるとキャッシュを使い直さない。トークン取得の転送先は Google の固定 URL とし、HTTP リダイレクトを追わない。

切り替え前に IAM・OAuth設定と Worker の版を記録する。障害時は、IAM 認証付きで動作確認した Worker 版へ戻す。
認証処理を持たない旧 Worker にだけ戻すと API に届かなくなる。匿名アクセスの再開は自動で行わず、運用者の判断が必要。

## 参考

- [Cloud Run のサービス間認証](https://docs.cloud.google.com/run/docs/authenticating/service-to-service)
- [Cloud Run の IAM チェック](https://docs.cloud.google.com/run/docs/authenticating/public)
- [Workers Secrets](https://developers.cloudflare.com/workers/configuration/secrets/)
