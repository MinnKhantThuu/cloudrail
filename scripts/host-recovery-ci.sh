#!/usr/bin/env bash
# Fence only Cloudrail fixtures on an explicitly disposable GitHub-hosted source runner.
set -euo pipefail
cd "$(dirname "$0")/.."
if [ "${GITHUB_ACTIONS:-}" != true ] || [ "${RUNNER_ENVIRONMENT:-}" != github-hosted ] || [ "${CLOUDRAIL_DISPOSABLE_TEST:-}" != 1 ]; then
  echo 'This source-fencing step requires an explicitly disposable GitHub-hosted runner.' >&2
  exit 1
fi
# Stop reconciliation before removing fixtures, or the agent can recreate them.
bash scripts/compose.sh stop agent server
mapfile -t task_apps < <(docker ps -aq --filter label=cloudrail.managed=true)
if [ "${#task_apps[@]}" -gt 0 ]; then docker rm -f "${task_apps[@]}" >/dev/null; fi
bash scripts/compose.sh down
if [ -n "$(docker ps -q --filter label=cloudrail.managed=true)" ] || [ -n "$(docker ps -q --filter label=com.docker.compose.project=cloudrail)" ]; then
  echo 'Source workload fencing failed' >&2
  exit 1
fi
echo 'PASS source workloads fenced before transfer to the independent destination runner'
