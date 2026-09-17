#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)/backend-go"

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "gofmt needed for:"
  echo "$unformatted"
  exit 1
fi

go vet ./...
go build ./...
go test ./...
