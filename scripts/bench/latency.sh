#!/usr/bin/env bash
# HTTP エンドポイントのレイテンシを測り、生データを CSV に残して要約を出す。
#
#   usage: scripts/bench/latency.sh <url> [count] [label]
#   例:    scripts/bench/latency.sh https://example.com/shops 100 api-render-shops
#
# 生データ: docs/benchmarks/raw/<label>.csv
set -euo pipefail

url=${1:?usage: latency.sh <url> [count] [label]}
count=${2:-100}
label=${3:-bench-$(date +%Y%m%d-%H%M%S)}

out_dir=${BENCH_OUT:-docs/benchmarks/raw}
out="${out_dir}/${label}.csv"
mkdir -p "$out_dir"

{
  echo "# url=${url}"
  echo "# measured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "n,total_s,connect_s,http_code"
} > "$out"

for i in $(seq 1 "$count"); do
  res=$(curl -sS -o /dev/null \
    -w '%{time_total},%{time_connect},%{http_code}' \
    --max-time 30 "$url" 2>/dev/null || echo "0,0,000")
  echo "${i},${res}" >> "$out"
done

awk -F, '
  /^#/ { next }
  NR > 1 && $4 ~ /^2/ { printf "%.1f\n", $2 * 1000 }
' "$out" | sort -n | awk -v label="$label" '
  { a[NR] = $1 }
  END {
    if (NR == 0) { print label ": 成功したリクエストがありません"; exit 1 }
    p50 = a[int(NR * 0.50) + 1]; if (p50 == "") p50 = a[NR]
    p95 = a[int(NR * 0.95) + 1]; if (p95 == "") p95 = a[NR]
    printf "%s: n=%d  p50=%.0fms  p95=%.0fms  min=%.0fms  max=%.0fms\n",
      label, NR, p50, p95, a[1], a[NR]
  }
'

echo "raw: $out"
