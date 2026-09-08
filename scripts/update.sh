#!/usr/bin/env bash
# Run after copying a reviewed source release over an existing installation.
set -euo pipefail
cd "$(dirname "$0")/.."
umask 077
if [ ! -f deploy/local/.env ]; then echo 'No existing installation. Use dev-up.sh or install-vps.sh.' >&2; exit 1; fi
task_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
task_dir=".data/updates/$task_stamp"
mkdir -p "$task_dir"
for task_service in server agent; do
  task_image="$(docker inspect "cloudrail-$task_service-1" --format '{{.Image}}')"
  docker tag "$task_image" "cloudrail-$task_service:previous-$task_stamp"
done
cat > "$task_dir/previous-images.yaml" <<YAML
services:
  server:
    image: cloudrail-server:previous-$task_stamp
  agent:
    image: cloudrail-agent:previous-$task_stamp
YAML
echo "Previous runtime images retained; recovery record: $task_dir"
# Failure here leaves the currently running containers untouched.
bash scripts/compose.sh build server agent
# Freeze job writers before the database/identity snapshot and schema update.
bash scripts/compose.sh stop agent server
if ! bash scripts/backup-control-plane.sh "$task_dir/control-backup"; then
  echo 'Backup failed; restarting previous runtime without applying migrations.' >&2
  bash scripts/compose.sh -f "$task_dir/previous-images.yaml" up -d --no-build --wait --wait-timeout 90 server agent
  exit 1
fi
if ! bash scripts/compose.sh up -d --no-build --wait --wait-timeout 180 server agent; then
  echo "Update did not become healthy. Keep $task_dir; follow docs/release.md recovery. Do not blindly downgrade the database." >&2
  exit 1
fi
cp VERSION "$task_dir/version"
echo 'Control plane updated. Verify node heartbeat, application routes and one deployment before ending maintenance.'
