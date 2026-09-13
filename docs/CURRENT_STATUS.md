# 7GRecorder Current Status

Last updated: 2026-09-13

This file is the handoff entry point for a new coding chat. Read it before reconstructing context from screenshots or
server commands.

Recommended first-read order:

1. `docs/CURRENT_STATUS.md`
2. `docs/AI_DEVELOPMENT_WORKFLOW.md`
3. `docs/ARCHITECTURE.md`, `docs/REQUIREMENTS.md`, and the task-specific docs listed in `AGENTS.md`

## Production

- Current deployed production commit: `96dcc61fd4008311a516d9b7a4b2be1f029c59b6`.
- `main` runs the reusable CI gate before the release job; `dev` runs CI only.
- The current release passed backend format, tidy, vet, tests, build, clean-database migration smoke, frontend
  lint/typecheck/tests/build, Compose validation, and production deployment.
- CI runs `34730687766` (dev) and `34730783719` (main) passed the complete repository gate. Production Deploy run
  `34730783854` completed successfully.
- Production includes safe delivered-source cleanup, review/module resume guards, Bilibili/COS progress reporting,
  direct original-part COS upload, safe interrupted-job recovery, and verified BililiveRecorder room-config sync.
- Normal backend deployment recreates only `7grecorder`. It must not use `docker compose down` or restart the
  independently recording `bililiverecorder` container.

## Bilibili And COS Delivery

- The Bilibili adapter presents multipart inputs through per-job aliases named `p01.<ext>`, `p02.<ext>`, and so on,
  so new Bilibili submissions display concise part names such as `p01`, `p02`, and `p03`.
- These aliases are symlinks inside the existing restricted biliup job directory. They neither copy multi-GB media
  nor rename canonical upload-source files.
- COS uploads each original publish part without FFmpeg transcoding or archive wrapping. New object keys use
  `videos/YYYY-MM-DD/session-NN/pNN.<source-format>` below the configured profile prefix.
- Existing remote COS objects retain their old names and formats; there is no automatic remote move or deletion.

## Implemented Recording And Upload Flow

- Active recordings appear in the admin recording list before an upload source exists.
- Completed source recordings are grouped into one parent upload source and packaged directly into upload parts.
- Packaging targets two-hour parts, while the configured maximum byte size remains authoritative. If the estimated
  two-hour part would exceed the byte limit, packaging first falls back to one hour and then uses a smaller safety
  estimate only for unusually high-bitrate input.
- Bilibili and COS have independent network workers and may progress concurrently.
- Upload progress is persisted on jobs and displayed in the admin console.
- Bilibili uses pinned `biliup==1.2.4`, defaults to upload concurrency `1`, Bilibili partition `2047`, and falls back
  to partition `27` only for an explicit partition rejection.
- COS uses the Tencent COS Go SDK, verifies a successful upload with `HeadObject`, and supports an independent
  request-body bandwidth cap.

## Upload Review And Editing

The review gate is implemented end to end:

1. An operator may mark an active recording or completed parent upload source as requiring review.
2. Local merge/package work may finish, but Bilibili and upload-source COS video uploads remain blocked.
3. If remote upload is already running, requiring review persists the gate, cancels the running jobs, resets remote
   state to pending, and cooperatively terminates the worker request.
4. While waiting for review, packaged parts can be downloaded directly from local storage by an authorized operator.
5. The edit form accepts one deletion interval per line in parent-timeline form, for example
   `00:10:00-00:12:30`.
6. Applying the edit creates replacement outputs under `upload-sources/<profile>/<source>/edited/` and keeps the
   source waiting for review.
7. A successful edit is visible when output paths use `edited/...`, the total duration/size is updated, and no edit
   decision remains pending. The `审核中` label is expected until the operator approves the result.
8. `审核完成` releases Bilibili and COS jobs. Both modules read the current `upload_source_outputs`, so edited paths
   are uploaded instead of the old `parts/...` paths.

Safety properties:

- Approval is rejected while an edit decision is still pending.
- Database triggers reject late Bilibili/COS success transitions while review is required.
- Running worker contexts observe persisted cancellation and terminate subprocess/network requests.
- Job completion updates only a job that is still `RUNNING`, preventing cancelled jobs from being overwritten.
- Reconcile cannot create or revive remote upload jobs while review is required or an edit decision is pending.

## Current Operator Workflow

For a recording that needs content removed:

1. Click `需要审核` as early as possible.
2. Wait for publish parts to appear and download the relevant local part for inspection.
3. Enter deletion intervals against the parent timeline and click `应用剪辑`.
4. Wait until filenames and paths change to `edited/...`; verify the new duration and download edited boundary parts.
5. Click `审核完成` only after the edited output is accepted.
6. Monitor Bilibili and COS jobs independently on the Jobs page.

Do not manually change upload-source, publication, COS-object, or job statuses in SQLite for the normal review flow.
Use the admin actions so cancellation, edit decisions, and downstream reset happen transactionally.

## Danmaku

- Recording configuration persists `record_danmaku` and exposes it in the admin profile editor.
- BililiveRecorder 2.18.0 room sync uses `POST /api/room/{roomId}/config` and the real `OptionalRecordDanmaku`
  request shape. A 200 response is accepted only when returned room values match the desired settings.
- Every Backend Worker startup requeues Recorder config sync, correcting settings previously reported as synced but
  ignored by BililiveRecorder. The server-side saved config and first XML created after a new file opens still need
  read-only production verification after release `96dcc61`.
- Raw danmaku files are indexed only when BililiveRecorder actually writes a closed danmaku asset.
- Closed raw danmaku assets are archived byte-for-byte to COS under the controlled `raw/` prefix.
- Parsing, merging, and aligning danmaku to edited/split video timelines remains out of scope pending real samples.

## Known Limitations And Next Work

- FFmpeg review edits currently use stream copy. Cut boundaries follow media keyframes and should be visually checked;
  frame-accurate re-encoding is not implemented.
- Bilibili has no true bytes-per-second limiter in the pinned CLI. `upload_limit=1` limits upload concurrency, not
  exact bandwidth.
- Bilibili verification/listing should later fill a missing BV URL when successful CLI output lacks an identifier.
- Danmaku timeline transformation and a richer browser media editor are still pending.

## Deployment And Interrupted Upload Recovery

- Deployment sets a temporary SQLite `worker_drain=true`, prevents new claims, and refuses to recreate the current
  container while it owns a real `RUNNING` job. Exit paths clear the drain; BililiveRecorder is never restarted.
- Each Worker process has a unique lock identity. At startup, orphaned local/COS jobs are reset for idempotent retry.
- An interrupted Bilibili upload is frozen as `Publication AMBIGUOUS` plus `Job FAILED`; it is never blindly retried.
  If its Publication was already `VERIFIED`, the orphaned Job is finalized as `SUCCEEDED` instead.
- Before retrying an ambiguous Bilibili job, verify in Creator Center that the same title/date submission does not
  exist. The admin confirmation sends `confirm_ambiguous_bilibili=true` and atomically reuses the existing
  Publication and Job.
- Release `96dcc61` still requires a read-only production state check before deciding whether any recovered
  Bilibili job should be retried. Do not edit SQLite statuses manually.

## Future Design Documents

- `docs/7GRecorder_Songs_V1_Technical_Design.md` defines the future evidence-driven Songs V1 pipeline.
- `docs/TencentCloud_SSL_Auto_Sync_Ubuntu_Nginx.md` describes a future Tencent Cloud SSL-to-Nginx sync design.
- These are design inputs only; neither feature is implemented by release `96dcc61`.

Do not introduce Redis, RabbitMQ, Kafka, PostgreSQL, or a workflow engine for these items without a new design review.
