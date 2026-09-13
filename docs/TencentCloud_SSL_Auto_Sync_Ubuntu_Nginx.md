# Tencent Cloud SSL Sync For 7g.chat

## 1. Status And Goal

This is the authoritative design for the production site domain and TLS certificate lifecycle.

Production names:

```text
7g.chat
www.7g.chat
```

The certificate is issued by Tencent Cloud SSL. 7GRecorder downloads renewed certificates with a dedicated CAM
sub-user credential entered through the SUPER_ADMIN web console. Host Nginx remains the public TLS endpoint.

## 2. Security Boundary

- The API credential is a `SYSTEM` credential with platform `tencent_ssl` and purpose `TLS`.
- The secret JSON contains `secret_id` and `secret_key`; it is encrypted with the existing 7GRecorder master key.
- Secret plaintext is write-only and is never returned by an API.
- The application container cannot reload host Nginx and cannot write `/etc/nginx`.
- The application may only stage a validated certificate below `/data/7grecorder/tls/7g.chat/pending`.
- A root-owned host service validates and installs staged files, runs `nginx -t`, and reloads Nginx.
- A failed download, validation, install, or config test must leave the currently active certificate in service.

Do not use the Tencent Cloud root-account key. Create a CAM sub-user dedicated to SSL synchronization. Its policy
must allow `DescribeCertificates` and `DownloadCertificate` for the intended certificate. `QcloudSSLFullAccess` is an
acceptable bootstrap policy; a narrower policy is preferred after the first successful deployment.

## 3. Runtime Flow

```text
SUPER_ADMIN saves Tencent SSL credential and site TLS settings
  -> durable SYNC_SITE_TLS job
  -> DescribeCertificates(domain=7g.chat, issued server certificates)
  -> choose the matching certificate with the latest expiry
  -> DownloadCertificate (base64 ZIP)
  -> validate key pair, SANs, and expiry in the application
  -> atomically stage fullchain.pem, privkey.pem, certificate-id
  -> root-owned systemd timer notices a new certificate-id
  -> validate key pair, SANs, and expiry again on the host
  -> install to /etc/nginx/ssl/7g.chat
  -> nginx -t
     -> success: reload Nginx and write deployed-certificate-id receipt
     -> failure: restore the previous files and keep Nginx running
  -> Worker observes the receipt and reports ACTIVE in the admin console
```

The Worker checks once at startup and every 30 seconds for due work. A successful remote check is scheduled again
after 24 hours. Saving settings or choosing "Sync now" reuses the singleton durable job and schedules it immediately.

## 4. Files

Application-owned staging area:

```text
/data/7grecorder/tls/7g.chat/pending/fullchain.pem
/data/7grecorder/tls/7g.chat/pending/privkey.pem
/data/7grecorder/tls/7g.chat/pending/certificate-id
/data/7grecorder/tls/7g.chat/deployed-certificate-id
```

Root-owned active files:

```text
/etc/nginx/ssl/7g.chat/fullchain.pem
/etc/nginx/ssl/7g.chat/privkey.pem
/etc/nginx/ssl/7g.chat/certificate-id
```

Nginx only reads the root-owned active files. The marker is written last during staging and deployment so readers do
not treat a partial write as a complete certificate.

## 5. Certificate Selection And Validation

The Tencent Cloud API version is `2019-12-05` and endpoint is `ssl.tencentcloudapi.com`.

`DescribeCertificates` uses `SearchKey=primary domain`, `CertificateStatus=[1]`, `CertificateType=SVR`, and
`ExpirationSort=DESC`.

Search results are not trusted as the final domain check. Before staging, 7GRecorder must:

- decode the returned ZIP with bounded size and safe paths;
- locate a PEM private key and certificate chain without relying on a fixed ZIP filename;
- parse the leaf certificate and private key as a pair;
- verify the primary domain and every configured additional domain with X.509 hostname rules;
- reject an expired or not-yet-valid leaf certificate;
- reject a selected certificate whose ID or metadata is missing.

The current production certificate shown in Tencent Cloud is `akxHVwKv`, covering `7g.chat` and `www.7g.chat`, and
expires on 2026-12-12. The implementation must still discover later renewal certificate IDs automatically.

## 6. Public Nginx Behavior

- Port 80 redirects `7g.chat` and `www.7g.chat` to HTTPS.
- Port 443 serves the SPA and proxies `/api/` and `/health/` to `127.0.0.1:8080`.
- `/internal/*` is never publicly proxied.
- `/_protected_media/` stays an Nginx `internal` location.
- Unknown hostnames and direct-IP HTTP requests do not become aliases for the production site.
- `APP_PUBLIC_BASE_URL` is `https://7g.chat`.

DNS A/AAAA records are an operator-owned prerequisite and are not changed by 7GRecorder.

## 7. Recovery And Operations

- The sync job classifies bad credentials as `AUTH`, malformed/unusable certificates as `PERMANENT`, and temporary
  API/network failures as `TRANSIENT`.
- An interrupted sync job is safe to retry because it only reads Tencent Cloud and writes a fixed staging target.
- Host deployment is idempotent by certificate ID.
- Never stop Nginx or BililiveRecorder because certificate synchronization failed.
- Logs and the admin status may include certificate ID, expiry, and error summaries, but never API secrets or private
  key contents.

First installation:

```bash
sudo bash /opt/7grecorder/current/source/scripts/deploy/install-nginx-domain-site.sh
```

Then add the Tencent SSL credential and enable Site TLS in the admin System page. Use "Sync now", wait for status
`ACTIVE`, and verify both `https://7g.chat/admin` and `https://www.7g.chat/admin`.
