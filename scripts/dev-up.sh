#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if ! docker info >/dev/null 2>&1; then
  echo 'Docker is not running. On macOS, start Docker Desktop or run: colima start' >&2
  exit 1
fi

if [ ! -f deploy/local/.env ]; then
  umask 077
  {
    echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"
    echo "CLOUDRAIL_AGENT_TOKEN=$(openssl rand -hex 32)"
  } > deploy/local/.env
fi
if ! grep -q '^CLOUDRAIL_ENCRYPTION_KEY=' deploy/local/.env; then
  umask 077
  echo "CLOUDRAIL_ENCRYPTION_KEY=$(openssl rand -hex 32)" >> deploy/local/.env
fi
if ! docker buildx version >/dev/null 2>&1; then
  # Standalone Homebrew Docker/Compose may not have the Buildx plugin installed.
  export DOCKER_BUILDKIT=0
fi
bash scripts/compose.sh up -d --build --wait --wait-timeout 90
echo 'Cloudrail: http://localhost:8080'
echo 'First visit: create your owner account directly in the browser. Later visits: sign in with your email/password.'
