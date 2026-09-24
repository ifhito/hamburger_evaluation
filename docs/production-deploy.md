# 本番へのデプロイ(CD)

main に入った変更を、GitHub Actions が自動で本番へ配ります。この文書は、その仕組みと、初回に一度だけ必要な準備をまとめたものです。

> **秘密の扱い**: ここに出てくる値(API トークン、DB の接続 URL など)は、すべて GitHub の **Production 環境**の Secrets に入れます。リポジトリ・PR・チャット・ログに、値そのものを書きません。この文書には**名前だけ**が載っています。

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
| `.github/workflows/deploy-frontend.yml` | main の `frontend/**` のうち、配る中身が変わったとき | 本番ビルド → 版をアップロード → プレビューで確かめる → 切り替える → 本番を確かめる |
| `.github/workflows/deploy-api.yml` | main の `backend-go/**` のうち、配る中身が変わったとき | マイグレーション → 要求を流さないリビジョンを作る → 専用の URL で確かめる → 切り替える → 本番を確かめる |

**テスト・Storybook・開発用の Docker 設定・文書だけの変更では走りません。** 配る中身が変わらないのに配り直しても、時間と無料枠を使うだけだからです。対象から外しているファイルは、各ワークフローの `paths` にある `!` の行です。テストだけを直した PR で本番を配り直したいときは、手動で実行します。

どちらも、Actions の画面から手動でも実行できます(`workflow_dispatch`)。**ただし main からしか配りません。** 手動実行で別のブランチを選んでも、次の 3 つのどれかで止まります。

1. ジョブの `if: github.ref == 'refs/heads/main'`
2. GitHub の Production 環境の、配信できるブランチの制限(main だけ)。値はこの環境に置いてあり、ほかのブランチからは読めない
3. GCP 側の入り口の条件(main の `deploy-api.yml` からだけ入れる。API のみ)

### 確かめてから切り替える

**壊れた版を本番に出さないため、新しい版を先に作って確かめてから、要求を切り替えます。**

| | 先に作る版 | 確かめる先 | 切り替え |
|---|---|---|---|
| フロントエンド | `wrangler versions upload`(まだ配信しない) | その版のプレビュー URL | `wrangler versions deploy <版>@100%` |
| API | `gcloud run deploy --no-traffic --tag candidate` | `candidate` の札が付いたリビジョン専用の URL | `gcloud run services update-traffic --to-revisions <リビジョン>=100` |

切り替えたあとにも本番を確かめ、落ちたら直前の版・リビジョンに自動で戻します。

フロントエンドの切り替え前の確認は、独自ドメイン(`burger-stack.com`)ではできません。独自ドメインは、切り替えるまで古い版を返すからです。版のプレビュー URL は workers.dev の上にしか作れないので、`wrangler.toml` の `preview_urls = true` で有効にしてあります。公開の入口としての workers.dev は止めて構いませんが、この設定は消さないでください。事前に確かめているので、ここで落ちることはまずありません。

確かめる中身は `.github/scripts/smoke-*.sh` にあります。**HTTP の 200 だけでは足りません。** 検証中、基盤の仮ページが 200 を返していたために、間違ったものを計測し続けたことが 2 度ありました。**中身の種類(Content-Type)と、中身の一部**まで確かめます。配った直後は伝播や起動が追いつかないことがあるので、5 秒おきに 5 回までやり直します。

- フロントエンド: `/` が HTML、`/api/meta` が API の JSON、`/api/auth/google/start` がページ遷移でも 302
- API: `/up` が `{"status":"ok"}`(DB まで通っている)、`/shops` が 200

フロントエンドの 3 つ目は、`Sec-Fetch-Mode: navigate` を付けて送ります。**ページ遷移のときだけ壊れる経路**があるためです(下の「既知の落とし穴」)。Worker の転送の約束(クエリ・本文・ヘッダー・302 の素通し)は、`frontend/worker/index.test.js` の単体テストでも固めています。

### マイグレーションが先

API のワークフローは、マイグレーションを当ててから、新しいイメージに入れ替えます。**古いコードが、新しいスキーマで動ける形の変更(前方互換)**であることが前提です。

列を消す・名前を変えるといった破壊的な変更は、2 回のリリースに分けます。1 回目で新しい形を足してコードを両対応にし、2 回目で古い形を消します。

## GitHub に入れる値

すべて、リポジトリの Settings → Environments → **Production** に入れます。リポジトリ全体の Secrets / Variables ではありません。Production 環境は、配信できるブランチを main だけに制限してあります。

### Secrets(値は秘密)

| 名前 | 中身 |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Cloudflare の API トークン。範囲は `hamburger-frontend` だけ、役割は Editor |
| `PROD_DATABASE_URL` | Neon の接続 URL。**pooler ではなく直結のほう** |

**Neon の URL は、pooler ではなく直結のものにします。** pooler 経由だと、マイグレーションが `unnamed prepared statement does not exist` で落ちることがあります(検証中に 2 回発生。再実行で通るが、当たり外れがある)。

### Variables(秘密ではない)

次の 4 つは必須です。秘密ではないので Variables に置きます(ログで伏せ字にならず、失敗したときに追いやすい)。GCP の 3 つは「2. GCP に、鍵を持たない入り口を作る」の最後に表示されます。

| 名前 | 中身 |
|---|---|
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare のアカウント ID。単体では何もできず、R2 のエンドポイントや画面の URL にも出る値 |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | `projects/<番号>/locations/global/workloadIdentityPools/github/providers/github` |
| `GCP_SERVICE_ACCOUNT` | `github-deployer@<プロジェクト ID>.iam.gserviceaccount.com` |
| `GCP_PROJECT_ID` | Google Cloud のプロジェクト ID |

次の 4 つは任意で、未設定なら既定値で動きます。

| 名前 | 既定値 |
|---|---|
| `FRONTEND_ORIGIN` | `https://burger-stack.com` |
| `API_ORIGIN` | `https://burger-stack-421794940461.asia-northeast1.run.app` |
| `CLOUD_RUN_SERVICE` | `burger-stack` |
| `CLOUD_RUN_REGION` | `asia-northeast1` |

独自ドメインを当てたら、`FRONTEND_ORIGIN` と `API_ORIGIN` を変え、`frontend/wrangler.toml` の `API_ORIGIN` も直します。

## 初回の準備

### 1. Cloudflare のトークンを作る

テンプレートの「Edit Cloudflare Workers」は使わず、**権限を絞ったカスタムトークン**を作ります。テンプレートには R2 の書き込み権限も入っていて、トークンが漏れると写真のバケットまで触れてしまうためです。

1. <https://dash.cloudflare.com/profile/api-tokens> を開き、**Create Token** → **Custom token** を選びます。
2. Workers の権限で、範囲を **Specified Workers**(特定の Worker)にして `hamburger-frontend` を選び、役割を **Editor** にします。

   | 範囲 | 役割 | できること |
   |---|---|---|
   | Specified Workers: `hamburger-frontend` | Editor | 既存の Worker の更新とデプロイ(静的ファイルを含む)。削除と、ほかの Worker への操作はできない |

   Cloudflare は 2026-09-15 に Workers の権限を役割ベースに変えました。以前の「Workers Scripts: Edit」は画面に出なくなっています([変更の告知](https://developers.cloudflare.com/changelog/post/2026-09-15-granular-worker-permissions/)、[役割の一覧](https://developers.cloudflare.com/workers/authorization/workers/))。

3. ほかの権限と Zone の範囲は付けません(独自ドメインのルートを使っていないため)。
4. 出てきたトークンを、GitHub の `CLOUDFLARE_API_TOKEN` に入れます(**この画面を閉じると二度と見られません**)。
5. アカウント ID は、Cloudflare のダッシュボードの右側にあります。GitHub の **Variables** の `CLOUDFLARE_ACCOUNT_ID` に入れます。

- 独自ドメインを当てたら、Zone の **Workers Routes: Edit** を足します。
- Editor は既存の Worker しか扱えません。Worker を作り直すときは、手元から `wrangler deploy` で一度作ってから CD に任せます。
- デプロイが認証エラーで落ちたら、Account の **Account Settings: Read** を足して試します。
- IP アドレスでの制限は付けません。GitHub Actions の実行環境は IP が毎回変わります。

### 2. GCP に、鍵を持たない入り口を作る

長く使えるサービスアカウントの鍵を GitHub に置かずに済む方法(Workload Identity Federation)を使います。流出しても悪用できる期間が数分に限られるためです。

Google Cloud コンソール右上の **Cloud Shell**(`gcloud` が入った端末)で、次を実行します。手元に `gcloud` を入れる必要はありません。`PROJECT_ID` と `REPO` は自分のものに置き換えます。

```bash
PROJECT_ID=<Google Cloud のプロジェクト ID>
REPO=ifhito/hamburger_evaluation
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')

# 使う API を有効にする(有効済みなら何も起きない)。
# iamcredentials と sts は、鍵なしで入る仕組みそのものに要る。
gcloud services enable --project="$PROJECT_ID" \
  iamcredentials.googleapis.com sts.googleapis.com \
  run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com

# 入り口(プール)と、GitHub からの身元を受け付ける設定
gcloud iam workload-identity-pools create github \
  --project="$PROJECT_ID" --location=global --display-name="GitHub Actions"

gcloud iam workload-identity-pools providers create-oidc github \
  --project="$PROJECT_ID" --location=global --workload-identity-pool=github \
  --issuer-uri=https://token.actions.githubusercontent.com \
  --attribute-mapping='google.subject=assertion.sub,attribute.repository=assertion.repository' \
  --attribute-condition="assertion.repository=='${REPO}' && assertion.ref=='refs/heads/main' && assertion.workflow_ref=='${REPO}/.github/workflows/deploy-api.yml@refs/heads/main'"

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
echo "GCP_PROJECT_ID=${PROJECT_ID}"
```

最後の 3 行に出た値を、GitHub の Production 環境の **Variables** に入れます(Settings → Environments → Production)。どれも秘密の値ではありません。`attribute-condition` は、**このリポジトリの main の `deploy-api.yml` 以外からは入れない**ようにするためのもので、省略できません。GitHub 側の制限を誰かが外しても、GCP 側で止まります。

すでに入り口を作ってある場合は、条件だけを次のコマンドで締め直します。

```bash
gcloud iam workload-identity-pools providers update-oidc github \
  --project="$PROJECT_ID" --location=global --workload-identity-pool=github \
  --attribute-condition="assertion.repository=='${REPO}' && assertion.ref=='refs/heads/main' && assertion.workflow_ref=='${REPO}/.github/workflows/deploy-api.yml@refs/heads/main'"
```

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

切り替え後の確認に落ちたときは、CD が自動で直前の版に戻します。それ以外で戻すときは、手元から次を実行します。

```bash
cd frontend
npx wrangler rollback          # 直前の版に戻す
npx wrangler deployments list  # それより前に戻すときは、版の一覧から選ぶ
```

問題のコミットを revert して main に入れても、CD がもう一度配ります。**手動実行で古いブランチを選んで戻すことはできません**(main からしか配らないため)。

### API

切り替え後の確認に落ちたときは、CD が自動で直前のリビジョンに戻します。それ以外で戻すときは、Cloud Shell から次を実行します。

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

- `docs/infrastructure.md` — 本番の構成の全体像(サービス・ドメイン・設定の置き場所・既知の課題)
- `docs/adr/` — 各サービスを選んだ理由
- `docs/google-login-setup.md` — Google でのサインインの準備
- `backend-go/Dockerfile` — 本番用(多段ビルド + distroless)。開発用は `Dockerfile.dev`
