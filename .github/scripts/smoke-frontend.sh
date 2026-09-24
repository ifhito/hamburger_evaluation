#!/usr/bin/env bash
# フロントエンドの配信を確かめる。使い方: smoke-frontend.sh <オリジン>
#
# HTTP の 200 だけでは足りない。基盤の仮ページも 200 を返すし、/api/* の転送が
# 壊れていても index.html が 200 で返る(検証で 2 回ともこれに引っかかった)。
# 中身の種類まで見る。配った直後は伝播が追いつかないことがあるので、数回やり直す。
set -euo pipefail
origin="${1:?オリジンを渡す}"
# 末尾の / は取り除く(変数に https://example.com/ と入っていると、"${origin}/" が // になり、
# 別の応答(307 など)が返って確認が誤って失敗する)。
origin="${origin%/}"

check() {
  # / が HTML を返すこと
  curl -fsS -o /dev/null -w '%{http_code} %{content_type}\n' "${origin}/" | grep -q '^200 text/html' || { echo "/ が HTML ではない"; return 1; }

  # /api/meta が API の JSON を返すこと(転送が生きている)
  local ct
  ct=$(curl -fsS -o /tmp/meta.json -w '%{content_type}' "${origin}/api/meta") || { echo "/api/meta に届かない"; return 1; }
  case "${ct}" in application/json*) ;; *) echo "/api/meta が JSON ではない: ${ct}"; return 1 ;; esac
  grep -q '"rating"' /tmp/meta.json || { echo "/api/meta の中身が違う"; return 1; }

  # ページ遷移でも /api/* が Worker に届くこと(Google ログインの経路)。
  # アセット配信に横取りされると 200 の index.html になり、Worker が 302 を追いかけても 200 になる。
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' \
    -H 'Sec-Fetch-Mode: navigate' -H 'Sec-Fetch-Dest: document' -H 'Accept: text/html' \
    "${origin}/api/auth/google/start")
  [ "${code}" = "302" ] || { echo "ページ遷移の /api/auth/google/start が 302 ではない: ${code}"; return 1; }
}

for attempt in 1 2 3 4 5; do
  if check; then echo "OK: ${origin}"; exit 0; fi
  echo "(${attempt}/5 回目が失敗。5 秒後にやり直す)"
  sleep 5
done
echo "NG: ${origin}"
exit 1
