#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
task_files=(-f deploy/local/compose.yaml)
if [ -f .data/public-installation ]; then task_files+=(-f deploy/vps/public.yaml); fi
if docker compose version >/dev/null 2>&1; then
  exec docker compose --env-file deploy/local/.env "${task_files[@]}" "$@"
elif command -v docker-compose >/dev/null 2>&1; then
  exec docker-compose --env-file deploy/local/.env "${task_files[@]}" "$@"
else
  echo 'Docker Compose is required. Install the Docker Compose plugin, then retry.' >&2
  exit 1
fi
