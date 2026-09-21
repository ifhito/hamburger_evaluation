#!/usr/bin/env bash
# Phase 4 で各ホストのダッシュボードに貼る環境変数を組み立てる。
#
#   usage: scripts/bench/deploy-env.sh <api-url> [frontend-url]
#   例:    scripts/bench/deploy-env.sh https://hamburger-api.onrender.com
#
# .env.bench / .env.storage / .env.mail から値を集め、KEY=VALUE の形で出す。
# 出力には秘密が含まれる。画面に出すだけで、ファイルには保存しない。
set -euo pipefail

api_url=${1:?usage: deploy-env.sh <api-url> [frontend-url]}
fe_url=${2:-$api_url}
root=$(cd "$(dirname "$0")/../.." && pwd)

for f in .env.bench .env.storage .env.mail; do
  [ -f "$root/backend-go/$f" ] || { echo "★ backend-go/$f がありません" >&2; exit 1; }
done
set -a
# shellcheck disable=SC1090
. "$root/backend-go/.env.bench"; . "$root/backend-go/.env.storage"; . "$root/backend-go/.env.mail"
set +a

# 採用した構成(ADR の第一候補)
db=${NEON_URL:?NEON_URL が要る}
photo_ep=${R2_ENDPOINT:?R2_ENDPOINT が要る}
photo_bk=${R2_BUCKET:?}
photo_ak=${R2_ACCESS_KEY_ID:?}
photo_sk=${R2_SECRET_ACCESS_KEY:?}
photo_pub=${R2_PUBLIC_BASE_URL:?}
smtp_host=${MAILJET_SMTP_HOST:?MAILJET_SMTP_HOST が要る}
smtp_port=${MAILJET_SMTP_PORT:?}
smtp_sec=${MAILJET_SMTP_SECURITY:?}
smtp_user=${MAILJET_SMTP_USER:?}
smtp_pass=${MAILJET_SMTP_PASSWORD:?}
mail_from=${MAILJET_MAIL_FROM:?}

# 秘密鍵は毎回作り直すと既存のトークンが無効になる。固定したいときは
# JWT_SECRET / OAUTH_TOKEN_SECRET を環境変数で先に与えること。
jwt=${JWT_SECRET:-$(openssl rand -hex 32)}
oauth_secret=${OAUTH_TOKEN_SECRET:-$(openssl rand -hex 32)}

cat <<VARS
# --- Phase 4 用の環境変数($api_url) ---
# DB は Neon、写真は Cloudflare R2、メールは Mailjet(各 ADR の第一候補)。
# 4 つのホストすべてに同じ値を入れること。DB やストレージまで変えると、
# 測っているのが API なのか他の層なのか切り分けられなくなる。

PORT=8080
APP_BASE_URL=$fe_url
DATABASE_URL=$db
DB_MAX_CONNS=5

JWT_SECRET=$jwt
JWT_TTL=24h

PHOTO_STORAGE=s3
PHOTO_S3_ENDPOINT=$photo_ep
PHOTO_S3_BUCKET=$photo_bk
PHOTO_S3_ACCESS_KEY_ID=$photo_ak
PHOTO_S3_SECRET_ACCESS_KEY=$photo_sk
PHOTO_PUBLIC_BASE_URL=$photo_pub

SMTP_HOST=$smtp_host
SMTP_PORT=$smtp_port
SMTP_SECURITY=$smtp_sec
SMTP_USER=$smtp_user
SMTP_PASSWORD=$smtp_pass
MAIL_FROM=$mail_from

OAUTH_ISSUER=$api_url
OAUTH_TOKEN_SECRET=$oauth_secret

STATS_WORKER_INTERVAL=1s
VARS

cat >&2 <<'NOTE'

--- 設定するときの注意 ---
* PORT はホストが上書きすることがある(Render / Cloud Run)。その場合は入れなくてよい。
* APP_BASE_URL はフロントの URL。Phase 6 までは API の URL で代用してよいが、
  確認メールのリンクがそこを指すことになる。
* OAUTH_ISSUER は API 自身の公開 URL。デプロイ後に確定するので、
  URL が決まってから再デプロイする必要がある。
* JWT_SECRET と OAUTH_TOKEN_SECRET は実行のたびに新しく作られる。
  4 ホストで揃えたいなら、先に export してからこのスクリプトを呼ぶこと。
NOTE
