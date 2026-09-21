#!/usr/bin/env bash
# 一定時間放置したあとの「初回リクエスト」応答時間を繰り返し測る。
# スリープする PaaS と scale-to-zero のサーバーレスを同じ土俵で比べるためのもの。
#
#   usage: scripts/bench/coldstart.sh <url> [wait_minutes] [trials] [label]
#   例:    scripts/bench/coldstart.sh https://example.com/healthz 30 5 cold-render
#
# 1 試行 = wait_minutes 待つ → 1 リクエスト。trials 回繰り返す。
# 30 分 x 5 回なら約 2.5 時間かかる。バックグラウンドで流すこと。
set -euo pipefail

url=${1:?usage: coldstart.sh <url> [wait_minutes] [trials] [label]}
wait_min=${2:-30}
trials=${3:-5}
label=${4:-cold-$(date +%Y%m%d-%H%M%S)}

out_dir=${BENCH_OUT:-docs/benchmarks/raw}
out="${out_dir}/${label}.csv"
mkdir -p "$out_dir"

{
  echo "# url=${url}"
  echo "# wait_minutes=${wait_min} trials=${trials}"
  echo "# measured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "trial,waited_min,total_s,http_code"
} > "$out"

for t in $(seq 1 "$trials"); do
  echo "[$(date +%H:%M:%S)] trial ${t}/${trials}: ${wait_min} 分待機中..."
  sleep $(( wait_min * 60 ))
  res=$(curl -sS -o /dev/null \
    -w '%{time_total},%{http_code}' \
    --max-time 120 "$url" 2>/dev/null || echo "0,000")
  echo "${t},${wait_min},${res}" >> "$out"
  echo "[$(date +%H:%M:%S)] trial ${t}: ${res}"
done

awk -F, '
  /^#/ { next }
  NR > 1 && $4 ~ /^2/ { printf "%.2f\n", $3 * 1000 }
' "$out" | sort -n | awk -v label="$label" '
  { a[NR] = $1 }
  END {
    if (NR == 0) { print label ": 成功した試行がありません"; exit 1 }
    mid = (NR % 2) ? a[int(NR/2) + 1] : (a[NR/2] + a[NR/2 + 1]) / 2
    printf "%s: trials=%d  median=%.0fms  min=%.0fms  max=%.0fms\n",
      label, NR, mid, a[1], a[NR]
  }
'

echo "raw: $out"
