#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
[[ "$(uname -s)-$(uname -m)" == Linux-x86_64 ]] || { echo 'Install Node 22.18.0 and pnpm 10.15.0 for your platform.' >&2; exit 1; }
if [[ ! -x "$project_root/.node-toolchain/bin/node" ]]; then
  staging_dir="$(mktemp -d)"
  trap 'rm -rf "$staging_dir"' EXIT
  curl -fsSL https://nodejs.org/dist/v22.18.0/node-v22.18.0-linux-x64.tar.xz -o "$staging_dir/node-v22.18.0-linux-x64.tar.xz"
  curl -fsSL https://nodejs.org/dist/v22.18.0/SHASUMS256.txt -o "$staging_dir/SHASUMS256.txt"
  (cd "$staging_dir" && awk '$2 == "node-v22.18.0-linux-x64.tar.xz"' SHASUMS256.txt | sha256sum --check --status)
  mkdir -p "$project_root/.node-toolchain"
  tar -xJf "$staging_dir/node-v22.18.0-linux-x64.tar.xz" -C "$project_root/.node-toolchain" --strip-components=1
fi
export PATH="$project_root/.node-toolchain/bin:$PATH"
[[ "$(node --version)" == v22.18.0 ]]
corepack enable
corepack prepare pnpm@10.15.0 --activate
cd "$project_root/frontend"
pnpm install --frozen-lockfile
