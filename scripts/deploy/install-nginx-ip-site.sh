#!/usr/bin/env bash
set -euo pipefail

if ! command -v nginx >/dev/null 2>&1; then
  echo "nginx is not installed. Install it first with: sudo apt-get install -y nginx" >&2
  exit 1
fi

test -d /etc/nginx/sites-available || { echo "/etc/nginx/sites-available missing"; exit 1; }
test -d /etc/nginx/sites-enabled || { echo "/etc/nginx/sites-enabled missing"; exit 1; }
test -d /opt/7grecorder/current/frontend/dist || { echo "/opt/7grecorder/current/frontend/dist missing"; exit 1; }

cat >/etc/nginx/sites-available/7grecorder <<'EOF'
server {
    listen 80 default_server;
    server_name _;

    root /opt/7grecorder/current/frontend/dist;
    index index.html;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /health/ {
        proxy_pass http://127.0.0.1:8080;
    }

    location /_protected_media/ {
        internal;
        alias /data/7grecorder/;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF

rm -f /etc/nginx/sites-enabled/default
ln -sfn /etc/nginx/sites-available/7grecorder /etc/nginx/sites-enabled/7grecorder

nginx -t
systemctl reload nginx

echo "7GRecorder nginx IP access enabled on http://<server-ip>/admin"
