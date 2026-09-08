#!/usr/bin/env bash
# Explicit operator recovery: rotate bootstrap token and replace the node identity.
set -euo pipefail
cd "$(dirname "$0")/.."
if [ "${1:-}" != --replace-node ]; then echo 'Usage: scripts/node-reenroll.sh --replace-node (stops new work; running apps stay up)' >&2; exit 1; fi
bash scripts/backup-control-plane.sh
bash scripts/compose.sh stop agent
python3 - <<'PY'
import pathlib,secrets
p=pathlib.Path('deploy/local/.env');lines=p.read_text().splitlines();lines=[('CLOUDRAIL_AGENT_TOKEN='+secrets.token_hex(32)) if line.startswith('CLOUDRAIL_AGENT_TOKEN=') else line for line in lines];p.write_text('\n'.join(lines)+'\n');p.chmod(0o600)
PY
docker exec cloudrail-postgres-1 psql -U cloudrail -d cloudrail -v ON_ERROR_STOP=1 -c 'DELETE FROM nodes'
docker run --rm -v cloudrail_agent-state:/state alpine:3.22 sh -c 'rm -f /state/node.key /state/node.crt /state/ca.crt'
bash scripts/compose.sh up -d --no-build --force-recreate --wait server agent
echo 'Node re-enrollment started. Verify the fresh heartbeat in the dashboard.'
