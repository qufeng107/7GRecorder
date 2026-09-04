#!/usr/bin/env bash
set -euo pipefail

: "${ROLLBACK_SHA:?ROLLBACK_SHA is required}"

release_root="/opt/7grecorder/releases/${ROLLBACK_SHA}"
test -d "${release_root}" || { echo "release not found: ${ROLLBACK_SHA}"; exit 1; }

ln -sfn "${release_root}" /opt/7grecorder/current
cd /opt/7grecorder/deploy
GIT_SHA="${ROLLBACK_SHA}" docker compose up -d --no-deps 7grecorder
curl -fsS http://127.0.0.1:8080/health/ready >/dev/null
echo "${ROLLBACK_SHA}" > /opt/7grecorder/current-release
echo "rollback ok: ${ROLLBACK_SHA}"
