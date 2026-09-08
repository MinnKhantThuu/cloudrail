#!/usr/bin/env bash
# Supported target: an owner-operated Ubuntu 24.04/26.04 VPS, amd64 or arm64.
set -euo pipefail
umask 077
task_source="$(cd "$(dirname "$0")/.." && pwd)"
task_root=/opt/cloudrail
task_domain= task_apps= task_email= task_ip=
usage(){ echo 'sudo bash scripts/install-vps.sh --domain console.example.com --app-domain apps.example.com --email owner@example.com --public-ip IPV4 [--directory /opt/cloudrail] [--check-only]'; }
task_check=false
while [ $# -gt 0 ]; do
 case "$1" in
  --domain) task_domain="${2:?}"; shift 2;;
  --app-domain) task_apps="${2:?}"; shift 2;;
  --email) task_email="${2:?}"; shift 2;;
  --public-ip) task_ip="${2:?}"; shift 2;;
  --directory) task_root="${2:?}"; shift 2;;
  --check-only) task_check=true; shift;;
  --help|-h) usage; exit 0;;
  *) usage >&2; exit 1;;
 esac
done
if [ "$(uname -s)" != Linux ]; then echo 'Run this installer on the target Ubuntu VPS.' >&2; exit 1; fi
if [ "$EUID" -ne 0 ]; then echo 'Use sudo on the target VPS.' >&2; exit 1; fi
. /etc/os-release
if [ "$ID" != ubuntu ] || { [ "$VERSION_ID" != 24.04 ] && [ "$VERSION_ID" != 26.04 ]; }; then echo 'Supported installer targets: Ubuntu 24.04 or 26.04.' >&2; exit 1; fi
case "$(uname -m)" in x86_64|aarch64) ;; *) echo 'Unsupported CPU architecture' >&2; exit 1;; esac
for task_host in "$task_domain" "$task_apps"; do
 if [[ ! "$task_host" =~ ^[a-z0-9][a-z0-9.-]*\.[a-z][a-z0-9.-]*$ ]] || [[ "$task_host" == *..* ]]; then echo 'Use lowercase DNS hostnames without scheme or path.' >&2; exit 1; fi
done
if [ "$task_domain" = "$task_apps" ]; then echo 'Use separate dashboard and application domains.' >&2; exit 1; fi
if [[ ! "$task_email" =~ ^[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+$ ]]; then echo 'A valid ACME email is required.' >&2; exit 1; fi
if [[ ! "$task_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then echo 'A public IPv4 address is required.' >&2; exit 1; fi
if [[ "$task_root" != /* ]] || [ "$task_root" = / ] || [[ "$task_root" == *..* ]]; then echo 'Choose an absolute installation directory.' >&2; exit 1; fi
for task_host in "$task_domain" "cloudrail-preflight.$task_apps"; do
 if ! getent ahostsv4 "$task_host" | awk '{print $1}' | sort -u | grep -Fxq "$task_ip"; then echo "DNS for $task_host must point directly to $task_ip (configure the wildcard app record)." >&2; exit 1; fi
done
task_memory="$(awk '/MemTotal/ {print $2}' /proc/meminfo)"
if [ "$task_memory" -lt 3500000 ]; then echo 'Use a VPS with at least 4 GB RAM for builds and workloads.' >&2; exit 1; fi
task_disk_path=/var/lib/docker
if [ ! -d "$task_disk_path" ]; then task_disk_path="$task_root"; while [ ! -d "$task_disk_path" ]; do task_disk_path="$(dirname "$task_disk_path")"; done; fi
task_free="$(df -Pk "$task_disk_path" | awk 'NR==2 {print $4}')"
if [ "$task_free" -lt 8388608 ]; then echo 'At least 8 GB free Docker disk space is required; 20 GB is recommended.' >&2; exit 1; fi
if [ "$task_check" = true ]; then echo 'PASS OS, architecture, DNS, RAM and disk preflight. No packages or services changed.'; exit 0; fi
if ! command -v docker >/dev/null; then
 apt-get update
 apt-get install -y ca-certificates curl python3 openssl
 install -m 0755 -d /etc/apt/keyrings
 curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
 chmod a+r /etc/apt/keyrings/docker.asc
 cat > /etc/apt/sources.list.d/docker.sources <<APT
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $VERSION_CODENAME
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
APT
 apt-get update
 apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
 systemctl enable --now docker
fi
for task_command in python3 openssl; do if ! command -v "$task_command" >/dev/null; then echo "Install $task_command first." >&2; exit 1; fi; done
docker compose version >/dev/null
python3 - "$(docker version --format '{{.Server.APIVersion}}')" "$task_ip" <<'PY'
import ipaddress,sys
assert tuple(map(int,sys.argv[1].split('.'))) >= (1,47), 'Docker Engine API 1.47+ required'
assert ipaddress.ip_address(sys.argv[2]).is_global, 'Use a public IPv4 address'
PY
if docker inspect cloudrail-server-1 >/dev/null 2>&1 && [ ! -f "$task_root/deploy/local/.env" ]; then echo 'A different Cloudrail installation already exists; refusing to reuse its resources.' >&2; exit 1; fi
if ! docker inspect cloudrail-proxy-1 >/dev/null 2>&1; then
 if ss -ltnH '( sport = :80 or sport = :443 )' | grep -q .; then echo 'Ports 80/443 are already in use. Free them before installation.' >&2; exit 1; fi
fi
# A fresh AWS bootstrap contains only this explicit provisioning marker.
if [ -d "$task_root" ] && [ "$task_source" != "$(cd "$task_root" && pwd)" ] && [ ! -f "$task_root/deploy/local/compose.yaml" ]; then
 if find "$task_root" -mindepth 1 -maxdepth 1 ! -name PROVISIONING.txt -print -quit | grep -q .; then
  echo 'Installation directory contains unrelated files; choose an empty directory.' >&2; exit 1
 fi
fi
if [ -f "$task_root/deploy/local/.env" ]; then
 bash "$task_root/scripts/backup-control-plane.sh"
fi
mkdir -p "$task_root"
if [ "$task_source" != "$(cd "$task_root" && pwd)" ]; then
 # Copy source only; never replace installation credentials or local application data.
 tar -C "$task_source" --exclude=.git --exclude=.data --exclude=.cache --exclude=.tools --exclude=node_modules --exclude=dist --exclude=.env --exclude=test-results --exclude=playwright-report -cf - . | tar -C "$task_root" -xf -
fi
cd "$task_root"
if [ ! -f deploy/local/.env ]; then
 cat > deploy/local/.env <<ENV
POSTGRES_PASSWORD=$(openssl rand -hex 24)
CLOUDRAIL_AGENT_TOKEN=$(openssl rand -hex 32)
CLOUDRAIL_ENCRYPTION_KEY=$(openssl rand -hex 32)
ENV
fi
python3 - "$task_domain" "$task_apps" "$task_email" "$task_ip" <<'PY'
import pathlib,sys
p=pathlib.Path('deploy/local/.env');values=dict(line.split('=',1) for line in p.read_text().splitlines() if '=' in line)
values.update(dict(zip(['DASHBOARD_DOMAIN','APP_DOMAIN','ACME_EMAIL','PUBLIC_IP'],sys.argv[1:])))
p.write_text(''.join(k+'='+v+'\n' for k,v in values.items()));p.chmod(0o600)
PY
mkdir -p .data
python3 scripts/public_route.py "$task_domain" .data/controlplane.yaml
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml -f deploy/vps/public.yaml up -d --build --wait --wait-timeout 180
docker cp .data/controlplane.yaml cloudrail-agent-1:/routes/controlplane.yaml
# This marker makes operational scripts preserve the public overlay on future runs.
touch .data/public-installation
echo "Cloudrail started at https://$task_domain"
echo "Open https://$task_domain and create your owner account immediately."
echo 'Certificate issuance is asynchronous. Verify HTTPS before using the workspace.'
