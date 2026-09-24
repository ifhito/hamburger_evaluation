#!/usr/bin/env bash
# API の動作を確かめる。使い方: smoke-api.sh <オリジン>
#
# 200 だけでは足りない(基盤の仮ページも 200 を返す)。DB につながっていることまで見る。
# 新しいリビジョンは最初の要求で起動するので、数回やり直す。
set -euo pipefail
origin="${1:?オリジンを渡す}"
# 末尾の / は取り除く(変数に https://example.com/ と入っていると、"${origin}/" が // になり、
# 別の応答(307 など)が返って確認が誤って失敗する)。
origin="${origin%/}"

check() {
  local ct
  ct=$(curl -fsS -o /tmp/up.json -w '%{content_type}' "${origin}/up") || { echo "/up に届かない"; return 1; }
  case "${ct}" in application/json*) ;; *) echo "/up が JSON ではない: ${ct}"; return 1 ;; esac
  grep -q '"status":"ok"' /tmp/up.json || { echo "/up が ok ではない(DB に届いていない)"; return 1; }
  curl -fsS -o /dev/null -w '%{http_code}\n' "${origin}/shops" | grep -q '^200$' || { echo "/shops が 200 ではない"; return 1; }
}

for attempt in 1 2 3 4 5; do
  if check; then echo "OK: ${origin}"; exit 0; fi
  echo "(${attempt}/5 回目が失敗。5 秒後にやり直す)"
  sleep 5
done
echo "NG: ${origin}"
exit 1
