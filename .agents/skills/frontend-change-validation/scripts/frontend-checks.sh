#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)/frontend"
pnpm run type-check
pnpm run lint
pnpm run test
