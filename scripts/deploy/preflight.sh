#!/usr/bin/env bash
set -euo pipefail

: "${RELEASE_SHA:?RELEASE_SHA is required}"

test -d /opt/7grecorder || { echo "/opt/7grecorder missing"; exit 1; }
test -d /data/7grecorder || { echo "/data/7grecorder missing"; exit 1; }
test -r /etc/7grecorder/app.env || { echo "/etc/7grecorder/app.env missing"; exit 1; }
sudo test -s /etc/7grecorder/master.key || { echo "/etc/7grecorder/master.key missing"; exit 1; }
docker version >/dev/null
df -Pk /data/7grecorder >/dev/null

echo "preflight ok for ${RELEASE_SHA}"
