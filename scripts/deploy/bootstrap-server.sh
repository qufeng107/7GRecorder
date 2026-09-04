#!/usr/bin/env bash
set -euo pipefail

install -d -m 0755 /opt/7grecorder/releases
install -d -m 0755 /opt/7grecorder/deploy
install -d -m 0755 /etc/7grecorder
install -d -m 0755 /data/7grecorder/db
install -d -m 0755 /data/7grecorder/recordings
install -d -m 0755 /data/7grecorder/songs
install -d -m 0755 /data/7grecorder/temp
install -d -m 0755 /data/7grecorder/backups/db

if [ ! -f /etc/7grecorder/master.key ]; then
  umask 077
  openssl rand -base64 32 > /etc/7grecorder/master.key
fi
chmod 0600 /etc/7grecorder/master.key

if [ ! -f /etc/7grecorder/app.env ]; then
  cat > /etc/7grecorder/app.env <<'EOF'
APP_LISTEN_ADDR=:8080
APP_PUBLIC_BASE_URL=https://recorder.example.com
DATA_ROOT=/data/7grecorder
SQLITE_PATH=/data/7grecorder/db/7grecorder.db
TEMP_ROOT=/data/7grecorder/temp
RECORDER_BASE_URL=http://bililiverecorder:2356
RECORDER_BASIC_USER=change-me
RECORDER_BASIC_PASSWORD=change-me
MASTER_KEY_PATH=/etc/7grecorder/master.key
LOG_LEVEL=info

APP_PORT=8080
BILILIVE_RECORDER_IMAGE=example.invalid/bililiverecorder:pinned-version
EOF
  chmod 0640 /etc/7grecorder/app.env
fi

if [ ! -f /etc/7grecorder/recorder.env ]; then
  cat > /etc/7grecorder/recorder.env <<'EOF'
# BililiveRecorder runtime configuration belongs here.
# Keep the Recorder API private to localhost or the Docker network.
EOF
  chmod 0640 /etc/7grecorder/recorder.env
fi

docker version >/dev/null
df -h /data/7grecorder

echo "7GRecorder server bootstrap complete."
