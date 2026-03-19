#!/bin/bash
set -e

# サーバーの pid ファイルが残っている場合は削除（コンテナ再起動時のエラー防止）
rm -f /app/tmp/pids/server.pid

exec "$@"
