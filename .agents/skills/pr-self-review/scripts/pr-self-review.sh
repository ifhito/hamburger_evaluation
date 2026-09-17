#!/usr/bin/env bash
set -euo pipefail

git status --short --branch --untracked-files=all
printf '
--- git diff --check ---
'
git diff --check
printf '
--- git diff --stat ---
'
git diff --stat
if ! git diff --cached --quiet; then
  printf '
--- git diff --cached --stat ---
'
  git diff --cached --stat
fi
