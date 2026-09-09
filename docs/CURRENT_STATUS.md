# 7GRecorder Current Status

Last updated: 2026-09-09

This file is the handoff entry point for a new coding chat. Read it before reconstructing context from screenshots or
server commands.

Recommended first-read order for the next chat:

1. `docs/CURRENT_STATUS.md`
2. `docs/AI_DEVELOPMENT_WORKFLOW.md`
3. `docs/ARCHITECTURE.md`, `docs/REQUIREMENTS.md`, and the task-specific docs listed in `AGENTS.md`

## Production

- Current production commit: `244b71a006d81bb4af56f8786e99c3fc79344a83`
- Branch flow: `dev` runs CI only; `main` runs CI and Production Deploy.
- The production deploy for `244b71a` completed successfully.
- Normal backend deploy uses `docker compose --env-file /etc/7grecorder/app.env up -d --no-deps 7grecorder`.
- Normal backend deploy must not run `docker compose down` and must not recreate/restart BililiveRecorder.

## Recently Completed

Upload-source recovery was added after production drift caused by manual disk cleanup:

- `POST /api/v1/upload-sources/actions/repair`
- Admin UI button: `修复上传源`
- Periodic worker repair during upload-source discovery
- Missing derived merge/package files now roll the source back to `MERGE_PENDING` or `PACKAGE_PENDING` when original
  recording segment files still exist.
- If original segment files are missing, repair records a visible blocked state and cancels pending/failed downstream
  Bilibili/COS upload jobs instead of creating partial uploads.
- Existing failed/cancelled Bilibili and COS jobs can be reset when their source is repaired and ready again.
- Package reruns update `upload_source_outputs` by `(upload_source_id, sort_order)` instead of deleting rows, preserving
  COS object foreign-key references.
- CI passed on `dev` and `main` for `244b71a`.

## Known Production History

- 2026-09-05 recordings were intentionally marked deleted/source-missing and should not be uploaded to Bilibili.
- Some derived files under `/data/7grecorder/upload-sources` were manually deleted during disk recovery.
- Source `10` for 2026-09-08 lost its merge/package derived file but still had raw recordings at the time of repair
  planning, so it should be repairable by rerunning merge and package.
- Source `2` for 2026-09-06 lost old package parts but raw 2026-09-06 recordings existed, so it should also be
  repairable.
- Do not manually delete `upload-source-*.flv` files as a broad pattern. It can remove merge inputs or package outputs
  needed by Bilibili/COS retry paths.

## Immediate Test Plan

After refreshing the admin UI and confirming the footer version starts with `244b71a`:

1. Open `录像文件`.
2. Click `修复上传源`.
3. Click `扫描`, or wait for the worker's periodic scan.
4. Open `任务`.
5. Verify source `10` moves through `合并可上传视频` then `封装可上传视频`.
6. After package succeeds, verify Bilibili/COS upload tasks are recreated or reset instead of staying cancelled with
   `waiting for merge rerun`, `upload source has no package input`, or `source file is missing`.
7. Do not test with a dummy short video unless explicitly requested; the user wants formal uploads from real recordings.

If the repair button reports original source files missing, do not force upload. The correct next step is either restore
raw files from backup/COS if available, or mark that source terminal and skip Bilibili.

## Bilibili Upload Status

Implemented:

- Bilibili credential storage accepts a biliup-generated `cookies.json` JSON object.
- Admin UI supports title template, description template, tags, copyright type, and selected credential.
- Worker uses pinned `biliup==1.2.4` CLI in a per-job temp directory.
- Multi-P upload uses upload-source output parts.

Still pending:

- Verification/listing after upload to fill the final BV URL when the CLI does not print one.
- Better operator-facing error text for auth/category/platform rejection.
- Final production validation on a real 2026-09-08/2026-09-06 source after repair completes.

## COS Upload Status

Implemented:

- COS credential/profile storage.
- Upload-source output upload via Tencent COS Go SDK.
- COS download links from admin UI use signed URLs, not direct backend file downloads.
- COS compression before upload is supported with controlled preset metadata.
- Raw danmaku files are planned/partially designed as byte-for-byte COS archives without timeline transformation.

Important:

- Backend/server local download should not be the default public path.
- Future manager/public download rules should authorize signed COS URL generation, with per-user scope and rate limits.
- Super admin currently has full access.

## Disk And Cleanup

Already documented and partly implemented:

- Deployment-time housekeeping.
- Daily housekeeping timer.
- Cleanup targets include old release tarballs, old releases, old app Docker images/build cache, old DB backups, safe
  temp leftovers, apt cache, and bounded journal cleanup.
- Housekeeping must not delete BililiveRecorder workdir/config/current image, SQLite WAL/SHM directly, or business media
  files outside explicit business cleanup rules.

Next implementation to add:

- `UPLOAD_SOURCE_LOCAL_CLEANUP` or equivalent maintenance job.
- It may delete local merge/package derived files only after all enabled remote modules for that upload source are in a
  terminal state:
  - COS uploaded or terminally skipped/failed by policy.
  - Bilibili uploaded/verified or terminally skipped/failed by policy.
  - Raw danmaku archive uploaded or terminally skipped/failed by policy.
- It must keep DB metadata, remote object metadata, signed-download capability, and enough audit trail.
- It must never delete raw active/writing recordings.

## Recording Quality

Open question for later design:

- The current visible recording quality may be limited by source stream quality, BililiveRecorder stream selection,
  remux/copy behavior, or later compression/upload outputs.
- Before changing defaults, inspect actual BililiveRecorder config and a sample file with `ffprobe`.
- Prefer recording the highest available source stream without transcoding in the recording core.
- Any optional transcode/compression should be a derived upload artifact, not a destructive replacement of original
  recordings.

## Danmaku

Decision:

- Download/archive raw danmaku first.
- Upload raw danmaku to COS.
- Do not parse, merge, or align it to split video timelines until real raw samples are reviewed.

Future design:

- Define how danmaku timestamps map to recording sessions, merged upload sources, and package parts.
- Add UI download links and status columns for raw danmaku COS archive.

## Next Coding Tasks

Recommended order:

1. Validate production repair flow on source `10` and source `2`.
2. If repair succeeds, retry formal Bilibili upload for real packaged outputs.
3. Add Bilibili verification/listing to persist BV URLs.
4. Add local derived-file cleanup after all enabled uploads finish.
5. Add raw danmaku scan/COS upload end-to-end.
6. Investigate recording quality with actual BililiveRecorder config and `ffprobe`.

Do not add broad infrastructure such as Redis, RabbitMQ, Kafka, PostgreSQL, or a workflow engine for these tasks.
