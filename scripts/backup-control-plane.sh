#!/usr/bin/env bash
# Includes private keys. Keep the resulting directory encrypted and off the VPS.
set -euo pipefail
cd "$(dirname "$0")/.."
umask 077
if command -v sha256sum >/dev/null; then task_hash=(sha256sum); else task_hash=(shasum -a 256); fi
task_backup="${1:-.data/backups/control-$(date -u +%Y%m%dT%H%M%SZ)}"
if [ -e "$task_backup" ]; then echo 'Backup destination already exists' >&2; exit 1; fi
mkdir -p "$task_backup"
docker exec cloudrail-postgres-1 pg_dump -U cloudrail -d cloudrail -Fc > "$task_backup/database.dump"
cp deploy/local/.env "$task_backup/installation.env"
docker cp cloudrail-server-1:/var/lib/cloudrail/authority.pem "$task_backup/authority.pem"
for task_file in node.key node.crt ca.crt; do
  docker cp "cloudrail-agent-1:/var/lib/cloudrail/$task_file" "$task_backup/$task_file"
done
chmod 600 "$task_backup"/*
(cd "$task_backup" && "${task_hash[@]}" database.dump installation.env authority.pem node.key node.crt ca.crt > SHA256SUMS)
echo "Control-plane backup saved: $task_backup"
