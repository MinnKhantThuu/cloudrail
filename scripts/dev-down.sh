#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
bash scripts/compose.sh stop
echo 'Platform stopped. Data and deployed application containers are retained.'
