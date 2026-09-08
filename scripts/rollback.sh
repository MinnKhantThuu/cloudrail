#!/usr/bin/env bash
# Image-only rollback is allowed only when schema/host/environment still match.
set -euo pipefail
cd "$(dirname "$0")/.."
exec python3 scripts/maintenance.py rollback "$@"
