#!/usr/bin/env bash
# Run from a reviewed source release copied over the existing installation.
set -euo pipefail
cd "$(dirname "$0")/.."
exec python3 scripts/maintenance.py update "$@"
