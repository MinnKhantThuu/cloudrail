#!/usr/bin/env bash
# Fast local iteration without retaining a compiler image layer for every change.
set -euo pipefail
cd "$(dirname "$0")/.."
task_root="$PWD"
task_go="${CLOUDRAIL_GO:-$task_root/.tools/go/bin/go}"
if [ ! -x "$task_go" ]; then task_go="$(command -v go)"; fi
task_arch="$(docker info --format '{{.Architecture}}')"
case "$task_arch" in aarch64|arm64) task_arch=arm64;; x86_64|amd64) task_arch=amd64;; *) echo 'Unsupported Docker architecture' >&2; exit 1;; esac
mkdir -p .data/runtime .cache/go .cache/mod
export GOCACHE="$task_root/.cache/go" GOMODCACHE="$task_root/.cache/mod"
npm --prefix apps/web run build
CGO_ENABLED=0 GOOS=linux GOARCH="$task_arch" "$task_go" build -trimpath -ldflags='-s -w' -o .data/runtime/server ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH="$task_arch" "$task_go" build -trimpath -ldflags='-s -w' -o .data/runtime/agent ./cmd/agent
cp -R apps/web/dist .data/runtime/web-next
rm -rf .data/runtime/web
mv .data/runtime/web-next .data/runtime/web
cp deploy/install-build-tools.sh .data/runtime/install-build-tools.sh
cp VERSION .data/runtime/VERSION
cat > .data/runtime/Dockerfile <<'DOCKERFILE'
FROM alpine:3.22 AS base
RUN apk add --no-cache ca-certificates
FROM base AS server
RUN adduser -D -u 10001 cloudrail
COPY server /usr/local/bin/server
COPY web /web
RUN mkdir -p /var/lib/cloudrail && chown 10001:10001 /var/lib/cloudrail
USER cloudrail
COPY VERSION /usr/share/cloudrail/VERSION
ENTRYPOINT ["server"]
FROM moby/buildkit:v0.33.0 AS build-tools
FROM base AS agent
COPY --from=build-tools /usr/bin/buildctl /usr/local/bin/buildctl
COPY install-build-tools.sh /tmp/install-build-tools.sh
RUN sh /tmp/install-build-tools.sh && rm /tmp/install-build-tools.sh
COPY agent /usr/local/bin/agent
COPY VERSION /usr/share/cloudrail/VERSION
ENTRYPOINT ["agent"]
DOCKERFILE
if ! docker buildx version >/dev/null 2>&1; then export DOCKER_BUILDKIT=0; fi
docker build -t cloudrail-server --target server .data/runtime
docker build -t cloudrail-agent --target agent .data/runtime
if ! grep -q '^CLOUDRAIL_ENCRYPTION_KEY=' deploy/local/.env; then
  umask 077
  echo "CLOUDRAIL_ENCRYPTION_KEY=$(openssl rand -hex 32)" >> deploy/local/.env
fi
bash scripts/compose.sh up -d --no-build --wait --wait-timeout 90
