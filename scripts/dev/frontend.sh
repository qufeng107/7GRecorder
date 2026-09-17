#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PATH="$project_root/.node-toolchain/bin:$PATH"
if ! command -v node >/dev/null || [[ "$(node --version)" != v22.18.0 ]]; then
  echo 'Node 22.18.0 required. Run: bash scripts/dev/setup-node.sh' >&2
  exit 1
fi
cd "$project_root/frontend"
exec pnpm "${@:-dev:mock}"
