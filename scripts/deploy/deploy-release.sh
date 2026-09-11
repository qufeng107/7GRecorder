#!/usr/bin/env bash
set -euo pipefail

: "${RELEASE_TAR:?RELEASE_TAR is required}"
: "${RELEASE_SHA:?RELEASE_SHA is required}"

release_root="/opt/7grecorder/releases/${RELEASE_SHA}"

cleanup_old_deploy_artifacts() {
  local keep_releases="${KEEP_RELEASES:-3}"
  local current_sha=""
  if [ -f /opt/7grecorder/current-release ]; then
    current_sha="$(cat /opt/7grecorder/current-release)"
  fi

  local release_count=0
  local release_path=""
  for release_path in $(ls -1dt /opt/7grecorder/releases/* 2>/dev/null || true); do
    [ -d "${release_path}" ] || continue
    local release_sha
    release_sha="$(basename "${release_path}")"
    if [ "${release_sha}" = "${RELEASE_SHA}" ] || [ "${release_sha}" = "${current_sha}" ]; then
      continue
    fi
    release_count=$((release_count + 1))
    if [ "${release_count}" -gt "${keep_releases}" ]; then
      rm -rf "${release_path}"
    fi
  done

  local tar_count=0
  local tar_path=""
  for tar_path in $(ls -1t /opt/7grecorder/deploy/7grecorder-release-*.tar /opt/7grecorder/deploy/7grecorder-release-*.tar.gz 2>/dev/null || true); do
    [ -f "${tar_path}" ] || continue
    case "$(basename "${tar_path}")" in
      "7grecorder-release-${RELEASE_SHA}.tar"|"7grecorder-release-${RELEASE_SHA}.tar.gz")
        continue
        ;;
    esac
    tar_count=$((tar_count + 1))
    if [ "${tar_count}" -gt "${keep_releases}" ]; then
      rm -f "${tar_path}"
    fi
  done

  local image=""
  for image in $(docker image ls 7grecorder --format '{{.Repository}}:{{.Tag}}' 2>/dev/null || true); do
    case "${image}" in
      "7grecorder:${RELEASE_SHA}"|"7grecorder:${current_sha}"|"7grecorder:<none>")
        continue
        ;;
    esac
    docker image rm "${image}" >/dev/null 2>&1 || true
  done

  docker image prune -f >/dev/null 2>&1 || true
  docker builder prune -af --filter "until=24h" >/dev/null 2>&1 || true
}

mkdir -p "${release_root}" /opt/7grecorder/deploy /data/7grecorder/backups/db
cleanup_old_deploy_artifacts

sha256sum -c SHA256SUMS
tar -xf "${RELEASE_TAR}" -C "${release_root}"
mkdir -p "${release_root}/source"
if [ -f "${release_root}/source.tar.gz" ]; then
  tar -xzf "${release_root}/source.tar.gz" -C "${release_root}/source"
else
  tar -xf "${release_root}/source.tar" -C "${release_root}/source"
fi
bash "${release_root}/source/scripts/deploy/preflight.sh"
HOUSEKEEPING_DEPLOY_SHA="${RELEASE_SHA}" bash "${release_root}/source/scripts/deploy/housekeeping.sh"

if [ -f "/data/7grecorder/db/7grecorder.db" ]; then
  cp "/data/7grecorder/db/7grecorder.db" "/data/7grecorder/backups/db/predeploy-${RELEASE_SHA}.db"
fi

image_archive="${release_root}/7grecorder-image.tar.gz"
if [ -f "${image_archive}" ]; then
  docker load -i "${image_archive}"
  docker image inspect "7grecorder:${RELEASE_SHA}" >/dev/null
else
  runtime_image="${RUNTIME_IMAGE:-7grecorder-runtime:bookworm-20250811-biliup-1.2.4-v1}"
  if ! docker image inspect "${runtime_image}" >/dev/null 2>&1; then
    echo "runtime image ${runtime_image} missing; building it once on this server" >&2
    docker build \
      -f "${release_root}/source/Dockerfile.runtime" \
      --build-arg BILIUP_VERSION="${BILIUP_VERSION:-1.2.4}" \
      -t "${runtime_image}" \
      "${release_root}/source"
  fi

  test -x "${release_root}/bin/7grecorder" || { echo "backend binary missing from release"; exit 1; }
  docker build \
    -f "${release_root}/source/Dockerfile.app" \
    --build-arg RUNTIME_IMAGE="${runtime_image}" \
    -t "7grecorder:${RELEASE_SHA}" \
    "${release_root}"
fi

test -d "${release_root}/frontend/dist" || { echo "frontend dist missing from release"; exit 1; }
cp "${release_root}/source/deploy/compose.yaml" /opt/7grecorder/deploy/compose.yaml

cd /opt/7grecorder/deploy
GIT_SHA="${RELEASE_SHA}" docker compose --env-file /etc/7grecorder/app.env run --rm --no-deps 7grecorder migrate
GIT_SHA="${RELEASE_SHA}" docker compose --env-file /etc/7grecorder/app.env up -d --no-deps 7grecorder

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health/ready >/dev/null; then
    ln -sfn "${release_root}" /opt/7grecorder/current
    echo "${RELEASE_SHA}" > /opt/7grecorder/current-release
    cleanup_old_deploy_artifacts
    HOUSEKEEPING_DEPLOY_SHA="${RELEASE_SHA}" bash "${release_root}/source/scripts/deploy/housekeeping.sh"
    echo "deploy ok: ${RELEASE_SHA}"
    exit 0
  fi
  sleep 2
done

echo "7GRecorder did not become ready" >&2
exit 1
