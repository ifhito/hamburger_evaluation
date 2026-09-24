#!/usr/bin/env bash
# メール送信サービス 1 社分を検証する。
#
#   usage: scripts/bench/mail-verify.sh <label> <宛先メールアドレス>
#   例:    scripts/bench/mail-verify.sh resend you@example.com
#
# 環境変数(backend-go/.env.mail から読む。<LABEL> は大文字):
#   <LABEL>_SMTP_HOST / _SMTP_PORT / _SMTP_SECURITY / _SMTP_USER / _SMTP_PASSWORD / _MAIL_FROM
#
# 行うこと:
#   1. 本番イメージを、その SMTP に向けてローカルで起動
#   2. サインアップを実行して確認メールを送らせる
#   3. mail_deliveries テーブルを追い、sent になるまでの時間を測る
#   4. 冪等性(同じアドレスで 2 回目)を確認
#   5. 存在しないポートに向けたときの失敗の分類を確認
#
# 受信できたかどうかは自動判定できない。受信箱を目で見て記録すること。
set -uo pipefail

label=${1:?usage: mail-verify.sh <label> <宛先メールアドレス>}
to=${2:?usage: mail-verify.sh <label> <宛先メールアドレス>}
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/docs/benchmarks/phase3-$label.md"
port=8082
name="bench-mail-$label"

up=$(echo "$label" | tr '[:lower:]' '[:upper:]')
host=$(eval echo "\${${up}_SMTP_HOST:-}")
sport=$(eval echo "\${${up}_SMTP_PORT:-}")
sec=$(eval echo "\${${up}_SMTP_SECURITY:-}")
user=$(eval echo "\${${up}_SMTP_USER:-}")
pass=$(eval echo "\${${up}_SMTP_PASSWORD:-}")
from=$(eval echo "\${${up}_MAIL_FROM:-}")

if [ -z "$host" ] || [ -z "$sport" ] || [ -z "$from" ]; then
  echo "★ ${up}_SMTP_HOST / _SMTP_PORT / _MAIL_FROM が要る(backend-go/.env.mail)"
  exit 1
fi

: "${DATABASE_URL:?DATABASE_URL が要る(ローカルの DB を指す)}"
container_db=$(printf '%s' "$DATABASE_URL" | sed -E 's%@(localhost|127\.0\.0\.1)(:[0-9]+)?/%@host.docker.internal\2/%')

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
q() { psql "$DATABASE_URL" -t -A -c "$1" 2>/dev/null; }

start_api() {
  docker rm -f "$name" >/dev/null 2>&1
  docker run -d --name "$name" --add-host=host.docker.internal:host-gateway -p $port:8080 \
    -e DATABASE_URL="$container_db" -e JWT_SECRET="$(openssl rand -hex 32)" -e PORT=8080 \
    -e APP_BASE_URL="http://localhost:5173" \
    -e SMTP_HOST="$1" -e SMTP_PORT="$2" -e SMTP_SECURITY="${sec:-starttls}" \
    -e SMTP_USER="$user" -e SMTP_PASSWORD="$pass" -e MAIL_FROM="$from" \
    hamburger-api:prod >/dev/null 2>&1
  for _ in $(seq 1 20); do curl -sf -o /dev/null "http://localhost:$port/up" && return 0; sleep 1; done
  return 1
}

signup() {
  curl -s -o /dev/null -w '%{http_code}' -X POST "http://localhost:$port/signup" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"username\":\"$2\",\"password\":\"Password123!\",\"password_confirmation\":\"Password123!\"}"
}

: > "$out"
{
  echo "# Phase 3: $label"
  echo ""
  echo "- 計測日: $(date '+%Y-%m-%d %H:%M')"
  echo "- SMTP: \`$host:$sport\`(\`${sec:-starttls}\`)"
  echo "- 差出人: \`$from\`"
  echo "- 宛先: 実行ごとに + エイリアスで一意にしている(記録時は伏せる)"
  echo ""
} >> "$out"

# ---------- 1. 正常系 ----------
say "[1/3] $host:$sport へ送信"
if ! start_api "$host" "$sport"; then
  echo "  ★ API が起動しない"; docker logs "$name" 2>&1 | tail -5
  { echo "## 結果"; echo ""; echo "**API が起動しなかった。** 設定を確認すること。"; } >> "$out"
  docker rm -f "$name" >/dev/null 2>&1; exit 1
fi

# 宛先を実行ごとに一意にする。アプリの冪等キーは確認待ちの受付 id から作られるため、
# 同じアドレスで繰り返すと 2 回目以降は送信されず、記録も増えない(アプリは正しい)。
# Gmail などの + エイリアスは同じ受信箱に届くので、受け取りには影響しない。
case "$to" in
  *+*) to_run="$to" ;;
  *@*) to_run="${to%@*}+mb$(date +%s)@${to#*@}" ;;
  *)   to_run="$to" ;;
esac
echo "  宛先(この実行用): $to_run"

before=$(q "select count(*) from mail_deliveries")
# この実行より前の行を拾わないよう、基準の時刻を取っておく。
# 取らないと、直前に測った社の結果を読んでしまう。
mark=$(q "select coalesce(max(created_at), now() - interval '1 second')::text from mail_deliveries")
uniq="bench-$(date +%s)"
code=$(signup "$to_run" "$uniq")
echo "  POST /signup -> $code"

t0=$(date +%s)
status=""; fail=""; err=""
for _ in $(seq 1 30); do
  row=$(q "select status || '|' || coalesce(failure_kind,'') || '|' || coalesce(last_error,'') from mail_deliveries where created_at > '${mark}'::timestamptz order by created_at desc limit 1")
  status=${row%%|*}; rest=${row#*|}; fail=${rest%%|*}; err=${rest#*|}
  [ -z "$status" ] && { sleep 1; continue; }
  [ "$status" = "sent" ] && break
  [ "$status" = "failed" ] && break
  sleep 1
done
t1=$(date +%s)
echo "  mail_deliveries: status=$status failure_kind=$fail ($((t1-t0)) 秒)"

{
  echo "## 結果"
  echo ""
  echo "| 項目 | 結果 |"
  echo "|---|---|"
  echo "| \`POST /signup\` | $code |"
  echo "| \`mail_deliveries.status\` | \`$status\` |"
  echo "| 送信までの時間 | $((t1-t0)) 秒 |"
  if [ "$status" = "failed" ]; then
    echo "| 失敗の種類 | \`$fail\` |"
    echo "| 理由 | \`$err\` |"
  fi
  echo ""
} >> "$out"

# ---------- 2. 冪等性 ----------
say "[2/3] 冪等性(同じアドレスでもう 1 回)"
code2=$(signup "$to_run" "${uniq}b")
sleep 5
after=$(q "select count(*) from mail_deliveries")
added=$((after - before))
echo "  2 回目の POST /signup -> ${code2}、mail_deliveries の増加 = $added 行"
{
  echo "## 冪等性"
  echo ""
  echo "同じアドレスで 2 回サインアップした。"
  echo ""
  echo "| 項目 | 結果 |"
  echo "|---|---|"
  echo "| 2 回目の \`POST /signup\` | $code2 |"
  echo "| \`mail_deliveries\` の増加 | $added 行 |"
  echo ""
  echo "2 回目は \`already_registered\` の通知になる想定(種別が違うので行は増える)。"
  echo ""
} >> "$out"

# ---------- 3. ポートが塞がれている場合の挙動 ----------
say "[3/3] 到達できないポートに向けたときの分類"
docker rm -f "$name" >/dev/null 2>&1
mark2=$(q "select coalesce(max(created_at), now() - interval '1 second')::text from mail_deliveries")
if start_api "$host" "47777"; then
  signup "blocked-$(date +%s)@example.com" "blocked$(date +%s)" >/dev/null
  sleep 25
  row=$(q "select status || '|' || coalesce(failure_kind,'') || '|' || coalesce(left(last_error,120),'') from mail_deliveries where created_at > '${mark2}'::timestamptz order by created_at desc limit 1")
  bstatus=${row%%|*}; brest=${row#*|}; bfail=${brest%%|*}; berr=${brest#*|}
  echo "  status=$bstatus failure_kind=$bfail"
  {
    echo "## 到達できないときの挙動"
    echo ""
    echo "ポート 47777(確実に閉じている)に向けて送らせた。2525 は Mailjet などが"
    echo "正規に受け付けるため、遮断の模擬には使えない。"
    echo ""
    echo "| 項目 | 結果 |"
    echo "|---|---|"
    echo "| \`status\` | \`$bstatus\` |"
    echo "| \`failure_kind\` | \`$bfail\` |"
    echo "| 理由 | \`$berr\` |"
    echo ""
    echo "\`temporary\` なら再試行で直りうる、\`permanent\` なら直らないと判定されている。"
    echo ""
  } >> "$out"
fi

docker rm -f "$name" >/dev/null 2>&1

{
  echo "## 受信の確認(手で記録する)"
  echo ""
  echo "| 項目 | 結果 |"
  echo "|---|---|"
  echo "| 受信箱に届いたか | (記入) |"
  echo "| 送信から受信までの体感 | (記入) |"
  echo "| 迷惑メールに入ったか | (記入) |"
  echo "| 差出人の表示 | (記入) |"
  echo ""
  echo "## SPF / DKIM の設定"
  echo ""
  echo "| 項目 | 結果 |"
  echo "|---|---|"
  echo "| 独自ドメインが要るか | (記入) |"
  echo "| DNS レコードの本数 | (記入) |"
  echo "| 反映までの時間 | (記入) |"
  echo ""
  echo "## ハマった点"
  echo ""
  echo "(記入)"
} >> "$out"

say "完了: $out"
