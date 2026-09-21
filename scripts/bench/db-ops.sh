#!/usr/bin/env bash
# DB 単体の読み書きレイテンシを統計つきで測る。
#
#   usage: scripts/bench/db-ops.sh <label> <DATABASE_URL> [iterations]
#   例:    scripts/bench/db-ops.sh neon "$NEON_URL" 200
#
# アプリの HTTP 層を挟まず pgx で直接叩く。結果は
# docs/benchmarks/phase1-<label>.md の末尾に追記される。
set -euo pipefail

label=${1:?usage: db-ops.sh <label> <DATABASE_URL> [iterations]}
db_url=${2:?usage: db-ops.sh <label> <DATABASE_URL> [iterations]}
iters=${3:-200}

root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/docs/benchmarks/phase1-$label.md"

# コンテナの中からは host の localhost に届かない。
container_url=$(printf '%s' "$db_url" | sed -E 's%@(localhost|127\.0\.0\.1)(:[0-9]+)?/%@host.docker.internal\2/%')

echo "[$label] $iters 回ずつ計測中..."
table=$(docker run --rm --add-host=host.docker.internal:host-gateway \
  -v "$root/scripts/bench/dbops:/src" \
  -v "$root/.gocache:/go/pkg/mod" \
  -w /src \
  -e DATABASE_URL="$container_url" -e ITERATIONS="$iters" \
  golang:1.27 go run . 2>&1)

if ! echo "$table" | grep -q '^| 操作'; then
  echo "★ 失敗:"; echo "$table" | tail -10; exit 1
fi

{
  echo ""
  echo "## DB 単体の読み書き($iters 回ずつ、pgx で直接)"
  echo ""
  echo "アプリの HTTP 層を挟まない。DB とネットワークの往復だけが出る。"
  echo ""
  echo "$table" | grep -E '^\|'
  echo ""
  echo "計測: $(date '+%Y-%m-%d %H:%M')"
} >> "$out"

echo "$table" | grep -E '^\|'
echo "→ $out に追記"
