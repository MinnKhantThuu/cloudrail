#!/usr/bin/env bash
# Rehearses restoration into a separate temporary DB on the local PostgreSQL.
set -euo pipefail
cd "$(dirname "$0")/.."
if command -v sha256sum >/dev/null; then task_hash=(sha256sum); else task_hash=(shasum -a 256); fi
task_backup="${1:?Usage: scripts/verify-control-restore.sh BACKUP_DIRECTORY}"
(cd "$task_backup" && "${task_hash[@]}" -c SHA256SUMS >/dev/null)
task_db="cloudrail_restore_$(date +%s)_$RANDOM"
cleanup(){ docker exec cloudrail-postgres-1 dropdb -U cloudrail --if-exists "$task_db" >/dev/null; }
trap cleanup EXIT
docker exec cloudrail-postgres-1 createdb -U cloudrail "$task_db"
docker exec -i cloudrail-postgres-1 pg_restore -U cloudrail -d "$task_db" --exit-on-error < "$task_backup/database.dump"
docker exec cloudrail-postgres-1 psql -U cloudrail -d "$task_db" -v ON_ERROR_STOP=1 -Atc "SELECT 'restored projects='||count(*) FROM projects; SELECT 'restored deployments='||count(*) FROM deployments; SELECT 'restored migrations='||count(*) FROM schema_migrations;"
echo 'PASS independent database restore; original workspace untouched'
