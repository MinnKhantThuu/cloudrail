#!/usr/bin/env bash
# Destructive only to this explicitly disposable GitHub-hosted runner's fixture daemon.
set -euo pipefail
cd "$(dirname "$0")/.."
if [ "${GITHUB_ACTIONS:-}" != true ] || [ "${RUNNER_ENVIRONMENT:-}" != github-hosted ] || [ "${CLOUDRAIL_DISPOSABLE_TEST:-}" != 1 ]; then
  echo 'This daemon-loss rehearsal requires an explicitly disposable GitHub-hosted runner.' >&2
  exit 1
fi
task_root="$PWD"
python3 scripts/host-recovery-acceptance.py prepare
python3 scripts/host-recovery.py backup .data/host-backup
python3 scripts/host-recovery.py verify .data/host-backup
# No backup, keys, owner fixtures or volume contents are uploaded as CI artifacts.
mkdir -p .data/recovery-target
git archive HEAD | tar -x -C .data/recovery-target
# Fence source workloads before disconnecting their daemon and storage.
mapfile -t task_apps < <(docker ps -aq --filter label=cloudrail.managed=true)
if [ "${#task_apps[@]}" -gt 0 ]; then docker rm -f "${task_apps[@]}" >/dev/null; fi
bash scripts/compose.sh down
sudo systemctl stop docker.service docker.socket
sudo mkdir -p /var/lib/cloudrail-recovery-test /run/cloudrail-recovery-test
printf '{}\n' | sudo tee /run/cloudrail-recovery-test/daemon.json >/dev/null
sudo sh -c 'nohup dockerd --config-file=/run/cloudrail-recovery-test/daemon.json --host=unix:///run/cloudrail-recovery-test/docker.sock --data-root=/var/lib/cloudrail-recovery-test --exec-root=/run/cloudrail-recovery-test --pidfile=/run/cloudrail-recovery-test/dockerd.pid --bridge=none --containerd-namespace=cloudrail-recovery --containerd-plugins-namespace=cloudrail-recovery-plugins --default-address-pool=base=172.29.0.0/16,size=24 > /tmp/cloudrail-recovery-dockerd.log 2>&1 &'
export DOCKER_HOST=unix:///run/cloudrail-recovery-test/docker.sock
for task_attempt in $(seq 1 60); do
  if docker info >/dev/null 2>&1; then break; fi
  sleep 1
done
docker info >/dev/null
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/cloudrail-recovery-test
test -z "$(docker image ls -q)"
test -z "$(docker volume ls -q)"
python3 .data/recovery-target/scripts/host-recovery.py restore "$task_root/.data/host-backup" --source-fenced
cp .data/test-owner.json .data/host-fixture.json .data/recovery-target/.data/
python3 .data/recovery-target/scripts/host-recovery-acceptance.py verify
