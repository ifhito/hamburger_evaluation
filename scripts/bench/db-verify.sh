#!/usr/bin/env bash
# DB 候補 1 社分の検証を通しで行う。
#
#   usage: scripts/bench/db-verify.sh <label> <DATABASE_URL>
#   例:    scripts/bench/db-verify.sh neon "postgres://...?sslmode=require"
#
# 行うこと:
#   1. マイグレーション 12 本の適用(所要時間を計る)
#   2. fixture と計測用データの投入
#   3. 本番イメージをその DB に向けて起動
#   4. /up と /shops のレイテンシ計測
#   5. DB_MAX_CONNS の上限探索
#
# 前提: docker、psql、hamburger-api:prod のイメージ(Dockerfile.prod でビルド済み)
set -uo pipefail

label=${1:?usage: db-verify.sh <label> <DATABASE_URL>}
db_url=${2:?usage: db-verify.sh <label> <DATABASE_URL>}

root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/docs/benchmarks/phase1-$label.md"
port=8081
name="bench-api-$label"

# コンテナの中からは host の localhost に届かない。ローカル DB を検証するときだけ
# host.docker.internal に読み替える(リモートの DB では何も変わらない)。
container_url=$(printf '%s' "$db_url" | sed -E 's%@(localhost|127\.0\.0\.1)(:[0-9]+)?/%@host.docker.internal\2/%')
if [ "$container_url" != "$db_url" ]; then
  echo "ローカル DB を検出: コンテナ内では host.docker.internal を使う"
fi

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
record() { echo "$*" >> "$out"; }

: > "$out"
record "# Phase 1: $label"
record ""
record "- 計測日: $(date '+%Y-%m-%d %H:%M')"
record "- ホスト: \`$(echo "$db_url" | sed -E 's|.*@([^/]+)/.*|\1|')\`"
record ""

# ---------- 1. サーバーの版 ----------
say "[1/5] サーバーの版と gen_random_uuid() の確認"
ver=$(psql "$db_url" -t -A -c "select current_setting('server_version')" 2>&1 | head -1)
uuid_ok=$(psql "$db_url" -t -A -c "select gen_random_uuid()" 2>&1 | head -1)
echo "  PostgreSQL: $ver"
record "## サーバー"
record ""
record "| 項目 | 結果 |"
record "|---|---|"
record "| PostgreSQL の版 | $ver |"
if [[ "$uuid_ok" =~ ^[0-9a-f-]{36}$ ]]; then
  echo "  gen_random_uuid(): OK"
  record "| \`gen_random_uuid()\` | 使える |"
else
  echo "  ★ gen_random_uuid() が使えない: $uuid_ok"
  record "| \`gen_random_uuid()\` | **使えない**: $uuid_ok |"
fi
record ""

# ---------- 2. マイグレーション ----------
say "[2/5] マイグレーション 12 本の適用"
t0=$(date +%s)
mig=$(docker run --rm --add-host=host.docker.internal:host-gateway -v "$root/backend-go/db/migrations:/migrations:ro" migrate/migrate:v4.18.1 \
        -path /migrations -database "$container_url" up 2>&1)
rc=$?
t1=$(date +%s)
echo "$mig" | tail -5
record "## マイグレーション"
record ""
if [ $rc -eq 0 ]; then
  record "12 本すべてが**無改変で適用できた**。所要 $((t1-t0)) 秒。"
else
  record "**失敗した。** 出力:"
  record ""
  record '```'
  record "$mig"
  record '```'
fi
record ""

# ---------- 3. データ投入 ----------
say "[3/5] データ投入(fixture + 計測用)"
docker run --rm --network host -e DATABASE_URL="$container_url" \
  -v "$root/backend-go:/app" -w /app golang:1.27 go run ./cmd/seed >/dev/null 2>&1 \
  && echo "  fixture: OK" || echo "  fixture: スキップ(既に投入済みか、失敗)"
psql "$db_url" -v users=50 -v shops=50 -v burgers=200 -v reviews=1000 \
  -f "$root/scripts/bench/seed-bulk.sql" >/dev/null 2>&1 && echo "  計測用データ: OK" || echo "  ★ 計測用データの投入に失敗"
counts=$(psql "$db_url" -t -A -F'/' -c "select (select count(*) from shops),(select count(*) from burgers),(select count(*) from reviews)")
echo "  件数(shops/burgers/reviews): $counts"
record "## データ量"
record ""
record "shops / burgers / reviews = \`$counts\`"
record ""

# ---------- 4. レイテンシ ----------
say "[4/5] レイテンシ計測"
docker rm -f "$name" >/dev/null 2>&1
docker run -d --name "$name" -p $port:8080 --memory 1g --add-host=host.docker.internal:host-gateway \
  -e DATABASE_URL="$container_url" -e JWT_SECRET="$(openssl rand -hex 32)" -e PORT=8080 \
  -e APP_BASE_URL="http://localhost:5173" \
  -e SMTP_HOST=localhost -e SMTP_PORT=1025 -e SMTP_SECURITY=none -e MAIL_FROM=n@e.com \
  hamburger-api:prod >/dev/null 2>&1
sleep 5
if ! curl -sf -o /dev/null "http://localhost:$port/up"; then
  echo "  ★ API が起動しない。ログ:"; docker logs "$name" 2>&1 | tail -5
  record "## レイテンシ"; record ""; record "**API が起動しなかった。**"
else
  BENCH_OUT="$root/docs/benchmarks/raw" "$root/scripts/bench/latency.sh" "http://localhost:$port/up"    50 "db-$label-up"
  BENCH_OUT="$root/docs/benchmarks/raw" "$root/scripts/bench/latency.sh" "http://localhost:$port/shops" 50 "db-$label-shops"
  up=$(awk -F, '/^#/{next} NR>1 && $4 ~ /^2/ {printf "%.1f\n", $2*1000}' "$root/docs/benchmarks/raw/db-$label-up.csv" | sort -n | awk '{a[NR]=$1} END{print a[int(NR*0.5)+1]}')
  sh=$(awk -F, '/^#/{next} NR>1 && $4 ~ /^2/ {printf "%.1f\n", $2*1000}' "$root/docs/benchmarks/raw/db-$label-shops.csv" | sort -n | awk '{a[NR]=$1} END{print a[int(NR*0.5)+1]}')
  record "## レイテンシ(ローカルの Docker から。50 回の p50)"
  record ""
  record "| エンドポイント | p50 |"
  record "|---|---|"
  record "| \`GET /up\` | ${up}ms |"
  record "| \`GET /shops\` | ${sh}ms |"
  record ""
  record "ローカル DB の基準値は /up 2ms、/shops 3ms(\`phase0-baseline.md\`)。"
  record ""
fi

# ---------- 5. 接続数の上限 ----------
say "[5/5] DB_MAX_CONNS の上限探索"
record "## 接続数の上限"
record ""
record "| DB_MAX_CONNS | 結果 |"
record "|---|---|"
for n in 5 10 20 50; do
  docker rm -f "$name" >/dev/null 2>&1
  docker run -d --name "$name" -p $port:8080 --memory 1g --add-host=host.docker.internal:host-gateway \
    -e DATABASE_URL="$container_url" -e DB_MAX_CONNS="$n" -e JWT_SECRET="$(openssl rand -hex 32)" -e PORT=8080 \
    -e APP_BASE_URL="http://localhost:5173" \
    -e SMTP_HOST=localhost -e SMTP_PORT=1025 -e SMTP_SECURITY=none -e MAIL_FROM=n@e.com \
    hamburger-api:prod >/dev/null 2>&1
  sleep 5
  if curl -sf -o /dev/null "http://localhost:$port/up"; then
    # 同時に叩いて実際に張れるか見る
    fail=0
    for i in $(seq 1 "$n"); do curl -sf -o /dev/null "http://localhost:$port/shops" || fail=$((fail+1)) & done; wait
    if [ $fail -eq 0 ]; then echo "  $n: OK"; record "| $n | 起動・同時 $n リクエストとも成功 |"
    else echo "  $n: 一部失敗 ($fail)"; record "| $n | 起動は成功、同時リクエストで $fail 件失敗 |"; fi
  else
    err=$(docker logs "$name" 2>&1 | tail -2 | tr '\n' ' ')
    echo "  $n: ★ 起動失敗"
    record "| $n | **起動失敗**: $err |"
  fi
done
docker rm -f "$name" >/dev/null 2>&1

record ""
record "## 休止からの復帰(手動)"
record ""
record "放置してから 1 クエリ投げて計る。放置時間は候補ごとに異なる。"
record ""
record '```bash'
record "# 30 分放置後"
record "time psql \"\$DATABASE_URL\" -c 'select 1'"
record '```'
record ""
record "| 放置時間 | 復帰までの時間 | 手動操作の要否 |"
record "|---|---|---|"
record "| 30 分 | (記入) | (記入) |"
record ""
record "## ハマった点"
record ""
record "(記入)"

say "完了: $out"
