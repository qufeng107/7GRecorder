#!/usr/bin/env bash
set -euo pipefail

domain="7g.chat"
staged_dir="/data/7grecorder/tls/${domain}/pending"
active_dir="/etc/nginx/ssl/${domain}"
receipt="/data/7grecorder/tls/${domain}/deployed-certificate-id"
site_source="/opt/7grecorder/current/source/deploy/nginx/7grecorder.conf.example"
site_target="/etc/nginx/sites-available/7grecorder"

test -f "${staged_dir}/certificate-id" || exit 0
certificate_id="$(tr -d '\r\n' < "${staged_dir}/certificate-id")"
[[ "${certificate_id}" =~ ^[A-Za-z0-9_-]+$ ]] || { echo "invalid staged certificate id" >&2; exit 1; }

if [ -f "${active_dir}/certificate-id" ] && [ "$(tr -d '\r\n' < "${active_dir}/certificate-id")" = "${certificate_id}" ]; then
  printf '%s\n' "${certificate_id}" > "${receipt}"
  chmod 0644 "${receipt}"
  exit 0
fi

test -s "${staged_dir}/fullchain.pem"
test -s "${staged_dir}/privkey.pem"
test -f "${site_source}"

openssl x509 -in "${staged_dir}/fullchain.pem" -noout -checkend 86400
openssl x509 -in "${staged_dir}/fullchain.pem" -noout -checkhost "7g.chat" >/dev/null
openssl x509 -in "${staged_dir}/fullchain.pem" -noout -checkhost "www.7g.chat" >/dev/null

work_dir="$(mktemp -d /var/tmp/7grecorder-tls.XXXXXX)"
trap 'rm -rf -- "${work_dir}"' EXIT
openssl x509 -in "${staged_dir}/fullchain.pem" -pubkey -noout | openssl pkey -pubin -outform DER > "${work_dir}/cert.pub"
openssl pkey -in "${staged_dir}/privkey.pem" -pubout -outform DER > "${work_dir}/key.pub"
cmp -s "${work_dir}/cert.pub" "${work_dir}/key.pub" || { echo "staged certificate and key do not match" >&2; exit 1; }

install -d -m 0700 "${active_dir}"
if [ -f "${active_dir}/fullchain.pem" ]; then cp -a "${active_dir}/fullchain.pem" "${work_dir}/fullchain.pem.bak"; fi
if [ -f "${active_dir}/privkey.pem" ]; then cp -a "${active_dir}/privkey.pem" "${work_dir}/privkey.pem.bak"; fi
if [ -f "${active_dir}/certificate-id" ]; then cp -a "${active_dir}/certificate-id" "${work_dir}/certificate-id.bak"; fi
if [ -f "${site_target}" ]; then cp -a "${site_target}" "${work_dir}/site.bak"; fi

rollback() {
  if [ -f "${work_dir}/fullchain.pem.bak" ]; then install -m 0644 "${work_dir}/fullchain.pem.bak" "${active_dir}/fullchain.pem"; else rm -f "${active_dir}/fullchain.pem"; fi
  if [ -f "${work_dir}/privkey.pem.bak" ]; then install -m 0600 "${work_dir}/privkey.pem.bak" "${active_dir}/privkey.pem"; else rm -f "${active_dir}/privkey.pem"; fi
  if [ -f "${work_dir}/certificate-id.bak" ]; then install -m 0644 "${work_dir}/certificate-id.bak" "${active_dir}/certificate-id"; else rm -f "${active_dir}/certificate-id"; fi
  if [ -f "${work_dir}/site.bak" ]; then install -m 0644 "${work_dir}/site.bak" "${site_target}"; else rm -f "${site_target}"; fi
}

install -m 0644 "${staged_dir}/fullchain.pem" "${active_dir}/fullchain.pem"
install -m 0600 "${staged_dir}/privkey.pem" "${active_dir}/privkey.pem"
printf '%s\n' "${certificate_id}" > "${active_dir}/certificate-id"
chmod 0644 "${active_dir}/certificate-id"
install -m 0644 "${site_source}" "${site_target}"
ln -sfn "${site_target}" /etc/nginx/sites-enabled/7grecorder

if ! nginx -t; then
  rollback
  nginx -t || true
  echo "nginx configuration rejected the staged certificate; previous site remains active" >&2
  exit 1
fi

if ! systemctl reload nginx; then
  rollback
  nginx -t && systemctl reload nginx || true
  echo "nginx reload failed; previous site was restored" >&2
  exit 1
fi
printf '%s\n' "${certificate_id}" > "${receipt}"
chmod 0644 "${receipt}"
echo "deployed Tencent SSL certificate ${certificate_id} for ${domain}"
