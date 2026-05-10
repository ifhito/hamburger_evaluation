#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)/backend"
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
