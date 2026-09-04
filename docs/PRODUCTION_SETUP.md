# 7GRecorder — Production Setup Checklist

## GitHub Repository

Required repository secrets:

```text
PROD_SSH_HOST
PROD_SSH_USER
PROD_SSH_PORT
PROD_SSH_PRIVATE_KEY
PROD_SSH_KNOWN_HOSTS
```

`PROD_SSH_KNOWN_HOSTS` is the server host key record, for example:

```text
42.192.108.218 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG72tPCr+6iSFeWz1mD/o1mGZXJ0Z5fyRpsIPT+lW7eD
```

Application secrets stay on the production server, not in GitHub Actions.

## Server Bootstrap

Run once on the production server as a user with permission to create `/opt`, `/etc`, and `/data` paths:

```bash
sudo bash scripts/deploy/bootstrap-server.sh
```

Then edit:

```text
/etc/7grecorder/app.env
/etc/7grecorder/recorder.env
```

Before the first production deploy, replace:

```text
APP_PUBLIC_BASE_URL
RECORDER_BASIC_USER
RECORDER_BASIC_PASSWORD
APP_UID
APP_GID
BILILIVE_RECORDER_IMAGE
```

`BILILIVE_RECORDER_IMAGE` must be a pinned image tag, not a floating latest tag.

`APP_UID` and `APP_GID` should match the deploy user that owns `/data/7grecorder`; for the default Ubuntu cloud image this is usually `1000`.

Confirm:

```bash
sudo test -r /etc/7grecorder/master.key
sudo test -r /etc/7grecorder/app.env
sudo test -d /data/7grecorder/db
sudo test -d /opt/7grecorder/deploy
id -u
id -g
```

If the deploy user was just added to the `docker` group, log out of SSH and log in again before running deploy checks. The current shell does not receive the new group automatically.

## First Production Deploy

After `dev` CI is green:

1. Open a pull request from `dev` to `main`.
2. Wait for the required `ci` check to pass.
3. Merge the PR.
4. The `Production Deploy` workflow runs on the `main` push.

Normal deploy updates only the 7GRecorder app container and frontend release. It must not run `docker compose down` and must not restart BililiveRecorder.

The production workflow uploads a small release, not a Docker image tar. The release includes `source.tar` and the frontend `dist` built by GitHub Actions. The server then builds:

```text
7grecorder:<git-sha>
uses the bundled frontend/dist
```

This keeps SCP fast on slow GitHub-to-mainland links while preserving GitHub CI as the required quality gate.
