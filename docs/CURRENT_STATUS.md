# 7GRecorder Current Status

Last updated: 2026-09-12

This file is the handoff entry point for a new coding chat. Read it before reconstructing context from screenshots or
server commands.

Recommended first-read order:

1. `docs/CURRENT_STATUS.md`
2. `docs/AI_DEVELOPMENT_WORKFLOW.md`
3. `docs/ARCHITECTURE.md`, `docs/REQUIREMENTS.md`, and the task-specific docs listed in `AGENTS.md`

## Production

- Current verified production commit: `44883f8a28e8a9962cfd90b2fec2af355001d4bc`.
- `main` runs the reusable CI gate before the release job; `dev` runs CI only.
- The current release passed backend format, tidy, vet, tests, build, clean-database migration smoke, frontend
  lint/typecheck/tests/build, Compose validation, and production deployment.
- Normal backend deployment recreates only `7grecorder`. It must not use `docker compose down` or restart the
  independently recording `bililiverecorder` container.

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
- Raw danmaku files are indexed only when BililiveRecorder actually writes a closed danmaku asset.
- Closed raw danmaku assets are archived byte-for-byte to COS under the controlled `raw/` prefix.
- Parsing, merging, and aligning danmaku to edited/split video timelines remains out of scope pending real samples.

## Known Limitations And Next Work

- FFmpeg review edits currently use stream copy. Cut boundaries follow media keyframes and should be visually checked;
  frame-accurate re-encoding is not implemented.
- Bilibili has no true bytes-per-second limiter in the pinned CLI. `upload_limit=1` limits upload concurrency, not
  exact bandwidth.
- Bilibili verification/listing should later fill a missing BV URL when successful CLI output lacks an identifier.
- Local derived-file cleanup after all enabled destinations reach a terminal state is still pending.
- Danmaku timeline transformation and a richer browser media editor are still pending.

Do not introduce Redis, RabbitMQ, Kafka, PostgreSQL, or a workflow engine for these items without a new design review.
