#!/usr/bin/env bash
# 実 API + 実 DB に対する結合テストを、隔離した Docker の環境で走らせる。
# 使い方: mcp-server/scripts/integration.sh
# 開発用のスタックには触れない（compose のプロジェクト名は he-s38、ポートは公開しない）。
set -euo pipefail
cd "$(dirname "$0")/.."

# このテスト専用の使い捨ての鍵。ファイルにも残さない。
JWT_SECRET="$(openssl rand -hex 32)"
export JWT_SECRET
compose=(docker compose -f docker-compose.integration.yml)

cleanup() { "${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

"${compose[@]}" run --rm test
