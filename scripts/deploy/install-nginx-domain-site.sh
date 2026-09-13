#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run this installer with sudo" >&2
  exit 1
fi

command -v nginx >/dev/null 2>&1 || { echo "nginx is not installed" >&2; exit 1; }
command -v openssl >/dev/null 2>&1 || { echo "openssl is not installed" >&2; exit 1; }
test -d /etc/nginx/sites-available
test -d /etc/nginx/sites-enabled
test -f /opt/7grecorder/current/source/scripts/deploy/deploy-staged-tls.sh

deploy_user="${SUDO_USER:-ubuntu}"
deploy_group="$(id -gn "${deploy_user}")"
install -d -o "${deploy_user}" -g "${deploy_group}" -m 0700 /data/7grecorder/tls /data/7grecorder/tls/7g.chat

cat > /etc/systemd/system/7grecorder-site-tls.service <<'EOF'
[Unit]
Description=Deploy the staged 7GRecorder site TLS certificate to host Nginx
After=network-online.target nginx.service

[Service]
Type=oneshot
ExecStart=/usr/bin/env bash /opt/7grecorder/current/source/scripts/deploy/deploy-staged-tls.sh
EOF

cat > /etc/systemd/system/7grecorder-site-tls.timer <<'EOF'
[Unit]
Description=Check for a staged 7GRecorder site TLS certificate

[Timer]
OnBootSec=1min
OnUnitActiveSec=1min
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now 7grecorder-site-tls.timer
systemctl start 7grecorder-site-tls.service

echo "7GRecorder domain TLS installer is active. The existing site remains unchanged until a valid certificate is staged."
