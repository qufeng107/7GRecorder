#!/usr/bin/env bash
set -euo pipefail

KEEP_RELEASES="${KEEP_RELEASES:-3}"
KEEP_DB_BACKUPS="${KEEP_DB_BACKUPS:-14}"
TEMP_MIN_AGE_HOURS="${TEMP_MIN_AGE_HOURS:-72}"
DOCKER_BUILD_CACHE_UNTIL="${DOCKER_BUILD_CACHE_UNTIL:-24h}"
JOURNAL_VACUUM_TIME="${JOURNAL_VACUUM_TIME:-14d}"
JOURNAL_VACUUM_SIZE="${JOURNAL_VACUUM_SIZE:-200M}"
DRY_RUN="${HOUSEKEEPING_DRY_RUN:-0}"
DEPLOY_SHA="${HOUSEKEEPING_DEPLOY_SHA:-${RELEASE_SHA:-}}"

log() {
  printf '%s\n' "$*"
}

run_cmd() {
  if [ "${DRY_RUN}" = "1" ]; then
    printf '[dry-run] %s\n' "$*"
    return 0
  fi
  "$@"
}

remove_file() {
  local path="$1"
  [ -f "${path}" ] || return 0
  run_cmd rm -f -- "${path}"
}

remove_dir() {
  local path="$1"
  [ -d "${path}" ] || return 0
  run_cmd rm -rf -- "${path}"
}

is_under() {
  local path="$1"
  local root="$2"
  case "${path}" in
    "${root}"/*) return 0 ;;
    *) return 1 ;;
  esac
}

cleanup_releases() {
  local releases_root="/opt/7grecorder/releases"
  local deploy_root="/opt/7grecorder/deploy"
  local current_sha=""

  [ -d "${releases_root}" ] || return 0
  if [ -f /opt/7grecorder/current-release ]; then
    current_sha="$(cat /opt/7grecorder/current-release)"
  fi

  local kept=0
  local release_path=""
  for release_path in $(ls -1dt "${releases_root}"/* 2>/dev/null || true); do
    [ -d "${release_path}" ] || continue
    is_under "${release_path}" "${releases_root}" || continue

    local release_sha
    release_sha="$(basename "${release_path}")"
    if [ "${release_sha}" = "${current_sha}" ]; then
      continue
    fi
    if [ -n "${DEPLOY_SHA}" ] && [ "${release_sha}" = "${DEPLOY_SHA}" ]; then
      continue
    fi

    kept=$((kept + 1))
    if [ "${kept}" -gt "${KEEP_RELEASES}" ]; then
      log "removing old release ${release_path}"
      remove_dir "${release_path}"
    fi
  done

  [ -d "${deploy_root}" ] || return 0

  kept=0
  local tar_path=""
  for tar_path in $(ls -1t "${deploy_root}"/7grecorder-release-*.tar 2>/dev/null || true); do
    [ -f "${tar_path}" ] || continue
    is_under "${tar_path}" "${deploy_root}" || continue

    if [ -n "${DEPLOY_SHA}" ] && [ "$(basename "${tar_path}")" = "7grecorder-release-${DEPLOY_SHA}.tar" ]; then
      continue
    fi

    kept=$((kept + 1))
    if [ "${kept}" -gt "${KEEP_RELEASES}" ]; then
      log "removing old release tar ${tar_path}"
      remove_file "${tar_path}"
    fi
  done
}

cleanup_db_backups() {
  local backup_root="/data/7grecorder/backups/db"
  [ -d "${backup_root}" ] || return 0

  local kept=0
  local backup_path=""
  for backup_path in $(ls -1t "${backup_root}"/*.db 2>/dev/null || true); do
    [ -f "${backup_path}" ] || continue
    is_under "${backup_path}" "${backup_root}" || continue
    kept=$((kept + 1))
    if [ "${kept}" -gt "${KEEP_DB_BACKUPS}" ]; then
      log "removing old db backup ${backup_path}"
      remove_file "${backup_path}"
    fi
  done
}

cleanup_temp() {
  local temp_root="/data/7grecorder/temp"
  [ -d "${temp_root}" ] || return 0

  local min_age_minutes=$((TEMP_MIN_AGE_HOURS * 60))
  local temp_path=""
  while IFS= read -r temp_path; do
    [ -n "${temp_path}" ] || continue
    is_under "${temp_path}" "${temp_root}" || continue
    log "removing stale temp ${temp_path}"
    if [ -d "${temp_path}" ]; then
      remove_dir "${temp_path}"
    else
      remove_file "${temp_path}"
    fi
  done < <(find "${temp_root}" -mindepth 1 -maxdepth 1 -mmin "+${min_age_minutes}" -print 2>/dev/null || true)
}

cleanup_docker() {
  command -v docker >/dev/null 2>&1 || return 0

  local current_sha=""
  if [ -f /opt/7grecorder/current-release ]; then
    current_sha="$(cat /opt/7grecorder/current-release)"
  fi

  local image=""
  for image in $(docker image ls 7grecorder --format '{{.Repository}}:{{.Tag}}' 2>/dev/null || true); do
    case "${image}" in
      "7grecorder:${current_sha}"|"7grecorder:${DEPLOY_SHA}"|"7grecorder:<none>")
        continue
        ;;
    esac
    log "removing old app image ${image}"
    run_cmd docker image rm "${image}" >/dev/null 2>&1 || true
  done

  run_cmd docker container prune -f --filter "until=24h" >/dev/null 2>&1 || true
  run_cmd docker image prune -f >/dev/null 2>&1 || true
  run_cmd docker builder prune -af --filter "until=${DOCKER_BUILD_CACHE_UNTIL}" >/dev/null 2>&1 || true
}

cleanup_system() {
  if [ "$(id -u)" -eq 0 ] && command -v apt-get >/dev/null 2>&1; then
    run_cmd apt-get clean >/dev/null 2>&1 || true
  fi

  if [ "$(id -u)" -eq 0 ] && command -v journalctl >/dev/null 2>&1; then
    run_cmd journalctl --vacuum-time="${JOURNAL_VACUUM_TIME}" >/dev/null 2>&1 || true
    run_cmd journalctl --vacuum-size="${JOURNAL_VACUUM_SIZE}" >/dev/null 2>&1 || true
  fi
}

main() {
  log "7GRecorder housekeeping started"
  cleanup_releases
  cleanup_db_backups
  cleanup_temp
  cleanup_docker
  cleanup_system
  log "7GRecorder housekeeping complete"
}

main "$@"
