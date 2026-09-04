#!/usr/bin/env bash
set -euo pipefail

: "${RELEASE_TAR:?RELEASE_TAR is required}"
: "${RELEASE_SHA:?RELEASE_SHA is required}"

release_root="/opt/7grecorder/releases/${RELEASE_SHA}"
mkdir -p "${release_root}" /opt/7grecorder/deploy /data/7grecorder/backups/db

sha256sum -c SHA256SUMS
tar -xf "${RELEASE_TAR}" -C "${release_root}"
mkdir -p "${release_root}/source"
tar -xf "${release_root}/source.tar" -C "${release_root}/source"
bash "${release_root}/source/scripts/deploy/preflight.sh"

if [ -f "/data/7grecorder/db/7grecorder.db" ]; then
  cp "/data/7grecorder/db/7grecorder.db" "/data/7grecorder/backups/db/predeploy-${RELEASE_SHA}.db"
fi

docker build --build-arg GIT_SHA="${RELEASE_SHA}" -t "7grecorder:${RELEASE_SHA}" "${release_root}/source"

docker run --rm \
  -v "${release_root}/source/frontend:/work" \
  -w /work \
  node:22.18.0-bookworm \
  bash -lc "corepack enable && corepack prepare pnpm@10.15.0 --activate && pnpm install --no-frozen-lockfile && pnpm build"

mkdir -p "${release_root}/frontend"
cp -R "${release_root}/source/frontend/dist" "${release_root}/frontend/dist"
cp "${release_root}/source/deploy/compose.yaml" /opt/7grecorder/deploy/compose.yaml

cd /opt/7grecorder/deploy
GIT_SHA="${RELEASE_SHA}" docker compose --env-file /etc/7grecorder/app.env run --rm --no-deps 7grecorder migrate
GIT_SHA="${RELEASE_SHA}" docker compose --env-file /etc/7grecorder/app.env up -d --no-deps 7grecorder

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health/ready >/dev/null; then
    ln -sfn "${release_root}" /opt/7grecorder/current
    echo "${RELEASE_SHA}" > /opt/7grecorder/current-release
    echo "deploy ok: ${RELEASE_SHA}"
    exit 0
  fi
  sleep 2
done

echo "7GRecorder did not become ready" >&2
exit 1
