#!/usr/bin/env bash
# 写真ストレージ候補 1 社分を検証する。
#
#   usage: scripts/bench/storage-verify.sh <label> [iterations]
#
# 環境変数(.env.storage から読む):
#   <LABEL>_ENDPOINT / _BUCKET / _ACCESS_KEY_ID / _SECRET_ACCESS_KEY / _PUBLIC_BASE_URL
#
# backend-go の adapter/storage/s3.go と同じ組み立てで client を作るので、
# 「エンドポイントを差し替えるだけで動くか」がそのまま分かる。
set -euo pipefail

label=${1:?usage: storage-verify.sh <label> [iterations]}
iters=${2:-50}
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/docs/benchmarks/phase2-$label.md"

up=$(echo "$label" | tr '[:lower:]' '[:upper:]')
ep=$(eval echo "\${${up}_ENDPOINT:-}")
bk=$(eval echo "\${${up}_BUCKET:-}")
ak=$(eval echo "\${${up}_ACCESS_KEY_ID:-}")
sk=$(eval echo "\${${up}_SECRET_ACCESS_KEY:-}")
pb=$(eval echo "\${${up}_PUBLIC_BASE_URL:-}")

if [ -z "$ep" ] || [ -z "$bk" ] || [ -z "$ak" ] || [ -z "$sk" ]; then
  echo "★ ${up}_ENDPOINT / _BUCKET / _ACCESS_KEY_ID / _SECRET_ACCESS_KEY が要る"
  echo "  backend-go/.env.storage に書いて source すること"
  exit 1
fi

echo "[$label] $iters 回ずつ計測中..."
body=$(docker run --rm --add-host=host.docker.internal:host-gateway \
  -v "$root/scripts/bench/storageops:/src" -w /src \
  -e S3_ENDPOINT="$ep" -e S3_BUCKET="$bk" \
  -e S3_ACCESS_KEY_ID="$ak" -e S3_SECRET_ACCESS_KEY="$sk" \
  -e PUBLIC_BASE_URL="$pb" -e ITERATIONS="$iters" \
  golang:1.27 go run . 2>&1) || true

{
  echo "# Phase 2: $label"
  echo ""
  echo "- 計測日: $(date '+%Y-%m-%d %H:%M')"
  echo "- エンドポイント: \`$ep\`"
  echo "- バケット: \`$bk\`"
  echo ""
  echo "$body" | grep -vE '^go: (downloading|finding)'
  echo ""
  echo "## ハマった点"
  echo ""
  echo "(記入)"
} > "$out"

echo "$body" | grep -vE '^go: (downloading|finding)'
echo "→ $out"
