# 本番へのデプロイ(CD)

main に入った変更を、GitHub Actions が自動で本番へ配ります。この文書は、その仕組みと、初回に一度だけ必要な準備をまとめたものです。

> **秘密の扱い**: ここに出てくる値(API トークン、DB の接続 URL など)は、すべて GitHub の Secrets に入れます。リポジトリ・PR・チャット・ログに、値そのものを書きません。この文書には**名前だけ**が載っています。

## 本番の構成

デプロイ先は、20 候補を実際に動かして比べたうえで決めました(経緯は ADR と計測記録にあります)。

| 層 | サービス | 置き場所 | 決め手 |
|---|---|---|---|
| フロントエンド | Cloudflare Workers(Static Assets) | 全世界のエッジ | API への転送が 4 候補で最速。帯域が無制限 |
| API | Google Cloud Run | 東京(`asia-northeast1`) | 写真処理・SMTP・常駐ワーカーが全部動いた唯一のホスト |
| データベース | Neon(PostgreSQL) | シンガポール | 無料枠で十分。復帰も速い |
| 写真 | Cloudflare R2 | 全世界 | 転送料が無料 |
| メール | Resend | — | 送信ドメインの検証が必須で、なりすましに使われにくい |

フロントエンドは、`/api/*` への要求だけを Worker が API へ転送します(`frontend/worker/index.js`)。ブラウザから見ると、画面も API も同じオリジンなので、CORS の設定が要りません。

## CD の動き

| ワークフロー | いつ走るか | 何をするか |
|---|---|---|
| `.github/workflows/deploy-frontend.yml` | main の `frontend/**` が変わったとき | 本番ビルド → Cloudflare へ配る → 配信を確かめる |
| `.github/workflows/deploy-api.yml` | main の `backend-go/**` が変わったとき | マイグレーション → Cloud Run へ配る → 動作を確かめる |

どちらも、Actions の画面から手動でも実行できます(`workflow_dispatch`)。

### 配ったあとの確認

**HTTP の 200 だけでは足りません。** 検証中、基盤の仮ページが 200 を返していたために、間違ったものを計測し続けたことが 2 度ありました。どちらのワークフローも、**中身の種類(Content-Type)と、中身の一部**まで確かめます。

- フロントエンド: `/` が HTML、`/api/meta` が API の JSON、`/api/auth/google/start` がページ遷移でも 302
- API: `/up` が `{"status":"ok"}`(DB まで通っている)、`/shops` が 200

3 つ目のフロントエンドの確認は、`Sec-Fetch-Mode: navigate` を付けて送ります。**ページ遷移のときだけ壊れる経路**があるためです(下の「既知の落とし穴」)。

### マイグレーションが先

API のワークフローは、マイグレーションを当ててから、新しいイメージに入れ替えます。**古いコードが、新しいスキーマで動ける形の変更(前方互換)**であることが前提です。

列を消す・名前を変えるといった破壊的な変更は、2 回のリリースに分けます。1 回目で新しい形を足してコードを両対応にし、2 回目で古い形を消します。

## GitHub に入れる値

### Secrets(値は秘密)

| 名前 | 中身 |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Cloudflare の API トークン。権限は Workers Scripts の Edit と Account Settings の Read だけ |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare のアカウント ID |
| `PROD_DATABASE_URL` | Neon の接続 URL。**pooler ではなく直結のほう** |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | `projects/<番号>/locations/global/workloadIdentityPools/github/providers/github` |
| `GCP_SERVICE_ACCOUNT` | `github-deployer@<プロジェクト ID>.iam.gserviceaccount.com` |
| `GCP_PROJECT_ID` | Google Cloud のプロジェクト ID |

**Neon の URL は、pooler ではなく直結のものにします。** pooler 経由だと、マイグレーションが `unnamed prepared statement does not exist` で落ちることがあります(検証中に 2 回発生。再実行で通るが、当たり外れがある)。

### Variables(秘密ではない。未設定なら既定値で動く)

| 名前 | 既定値 |
|---|---|
| `FRONTEND_ORIGIN` | `https://hamburger-frontend.hito01010101.workers.dev` |
| `API_ORIGIN` | `https://burger-stack-421794940461.asia-northeast1.run.app` |
| `CLOUD_RUN_SERVICE` | `burger-stack` |
| `CLOUD_RUN_REGION` | `asia-northeast1` |

独自ドメインを当てたら、`FRONTEND_ORIGIN` と `API_ORIGIN` を変え、`frontend/wrangler.toml` の `API_ORIGIN` も直します。

## 初回の準備

### 1. Cloudflare のトークンを作る

テンプレートの「Edit Cloudflare Workers」は使わず、**権限を絞ったカスタムトークン**を作ります。テンプレートには R2 の書き込み権限も入っていて、トークンが漏れると写真のバケットまで触れてしまうためです。

1. <https://dash.cloudflare.com/profile/api-tokens> を開き、**Create Token** → **Custom token** を選びます。
2. 権限を次の 2 つだけにします。

   | 種類 | 権限 | レベル | 用途 |
   |---|---|---|---|
   | Account | Workers Scripts | Edit | Worker と静的ファイルのアップロード |
   | Account | Account Settings | Read | wrangler がアカウントの情報を読む |

3. **Account Resources** は、このアプリのアカウントだけに絞ります。**Zone Resources** は要りません(独自ドメインのルートを使っていないため)。
4. 出てきたトークンを、GitHub の `CLOUDFLARE_API_TOKEN` に入れます(**この画面を閉じると二度と見られません**)。
5. アカウント ID は、Cloudflare のダッシュボードの右側にあります。`CLOUDFLARE_ACCOUNT_ID` に入れます。

- 独自ドメインを当てたら、Zone の **Workers Routes: Edit** を足します。
- デプロイが認証エラーで落ちたら、User の **Memberships: Read** を足します(アカウント ID を渡していれば、通常は要りません)。
- IP アドレスでの制限は付けません。GitHub Actions の実行環境は IP が毎回変わります。

### 2. GCP に、鍵を持たない入り口を作る

長く使えるサービスアカウントの鍵を GitHub に置かずに済む方法(Workload Identity Federation)を使います。流出しても悪用できる期間が数分に限られるためです。

`gcloud` を入れたうえで、次を実行します。`PROJECT_ID` と `REPO` は自分のものに置き換えます。

```bash
PROJECT_ID=<Google Cloud のプロジェクト ID>
REPO=ifhito/hamburger_evaluation
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')

# 入り口(プール)と、GitHub からの身元を受け付ける設定
gcloud iam workload-identity-pools create github \
  --project="$PROJECT_ID" --location=global --display-name="GitHub Actions"

gcloud iam workload-identity-pools providers create-oidc github \
  --project="$PROJECT_ID" --location=global --workload-identity-pool=github \
  --issuer-uri=https://token.actions.githubusercontent.com \
  --attribute-mapping='google.subject=assertion.sub,attribute.repository=assertion.repository' \
  --attribute-condition="assertion.repository=='${REPO}'"

# 配る役のサービスアカウント
gcloud iam service-accounts create github-deployer \
  --project="$PROJECT_ID" --display-name="GitHub Actions deployer"

SA="github-deployer@${PROJECT_ID}.iam.gserviceaccount.com"
for role in roles/run.admin roles/cloudbuild.builds.editor \
            roles/artifactregistry.writer roles/storage.admin \
            roles/iam.serviceAccountUser roles/logging.viewer; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="serviceAccount:${SA}" --role="$role"
done

# このリポジトリからだけ、この役になれるようにする
gcloud iam service-accounts add-iam-policy-binding "$SA" \
  --project="$PROJECT_ID" --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github/attribute.repository/${REPO}"

echo "GCP_WORKLOAD_IDENTITY_PROVIDER=projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github/providers/github"
echo "GCP_SERVICE_ACCOUNT=${SA}"
```

最後の 2 行に出た値を、GitHub の Secrets に入れます。`attribute-condition` は、**このリポジトリ以外からは入れない**ようにするためのもので、省略できません。

### 3. Cloud Run の継続的デプロイは使わない

Cloud Run のコンソールには、リポジトリへの push で自動的にビルドする機能(継続的デプロイ。中身は Cloud Build のトリガー)があります。**これは無効にしておきます。** 2026-09-24 に無効化しました。

有効のままだと、配る経路が 2 本になります。

- トリガーはマイグレーションを当てずにコードだけ入れ替える。検証中の `password_digest` の事故は、この形で起きた
- GitHub Actions とトリガーの、後から走ったほうが本番になる。対象のブランチが違えば、古いコードに戻りうる
- ドキュメントだけの変更でもビルドが走り、Cloud Build の無料枠を使う

Cloud Run のサービスを作り直したときや、コンソールから「リポジトリからデプロイ」を選んだときに、また有効になることがあります。サービスの画面に継続的デプロイの表示が出ていないことを確かめてください。

### 4. API の環境変数は Cloud Run 側に置く

`JWT_SECRET`・`SMTP_*`・`OAUTH_*`・Google ログイン・R2 の鍵などは、**Cloud Run のサービスに設定したまま**にします。CD は、イメージだけを入れ替えて、環境変数には触れません。

こうしておくと、本番の秘密を GitHub に置かずに済みます。環境変数を変えるときは、Cloud Run のコンソールか `gcloud run services update` で直します。

## ロールバック

### フロントエンド

```bash
cd frontend
npx wrangler rollback          # 直前の版に戻す
```

または、問題のコミットを revert して main に入れると、CD がもう一度配ります。

### API

```bash
gcloud run revisions list --service=burger-stack --region=asia-northeast1
gcloud run services update-traffic burger-stack \
  --region=asia-northeast1 --to-revisions=<戻したいリビジョン>=100
```

**当てたマイグレーションは戻りません。** コードを戻しても、スキーマは新しいままです。だから前方互換が要ります。

## 既知の落とし穴

### ページ遷移のときだけ /api が壊れる

Cloudflare の Static Assets は、`not_found_handling = "single-page-application"` を設定していると、**ブラウザのページ遷移(`Sec-Fetch-Mode: navigate`)に対しては、Worker より先にアセット配信が働き**、`index.html` を返します。`fetch` で呼ぶ API は無事なので、**Google ログインのような、ページ遷移を伴う経路だけ**が 404 になります。

`wrangler.toml` の `run_worker_first = ["/api/*"]` が、これを止めています。消さないでください。

同じ症状の原因がもう 1 つあります。Worker の中の `fetch` は、既定で 3xx を**自分で追いかけて**しまい、API が返した 302 の行き先の中身を 200 として返します。`worker/index.js` の `redirect: "manual"` が、これを止めています。

**原因が 2 つあるので、片方だけ直しても症状が変わりません。** デプロイ後の確認に、ページ遷移の見出しを付けた要求を入れてあるのは、このためです。

### 統計の再計算ワーカーが止まる

バーガーの統計は、常駐のワーカーが計算し直します(`STATS_WORKER_INTERVAL`)。**Cloud Run は、要求が無い間はインスタンスを 0 にする**ので、誰も見ていない間はワーカーも止まります。

いまは、次に誰かが来たときの起動直後の 1 サイクルで、溜まった依頼がまとめて処理されます。実害は「しばらく誰も来ないと、統計の反映が遅れる」ことです。

確実に動かすなら、どちらかが要ります。

- Cloud Run の最小インスタンスを 1 にする(常時課金になる)
- 外部の cron から定期的に呼ぶ

**CD の範囲外**なので、このワークフローでは何もしていません。

### CI の通過は待たない

デプロイのワークフローは、`ci.yml` の完了を待ちません。**main に入る前に PR で検査が通っている**前提です。main への直接 push を禁じ、`ci.yml` と `backend-go.yml` を必須の検査にしておいてください。

ただし、壊れたものがそのまま配られるわけではありません。フロントエンドは本番ビルドが落ちれば配信前に止まり、API は Go のビルドが落ちれば Cloud Build が失敗します。

## 関連する文書

- `docs/google-login-setup.md` — Google でのサインインの準備
- `backend-go/Dockerfile` — 本番用(多段ビルド + distroless)。開発用は `Dockerfile.dev`
