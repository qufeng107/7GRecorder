# 7GRecorder Current Status

Last updated: 2026-09-19

This file is the handoff entry point for a new coding chat. Read it before reconstructing context from screenshots or
server commands.

Recommended first-read order:

1. `docs/CURRENT_STATUS.md`
2. `docs/AI_DEVELOPMENT_WORKFLOW.md`
3. `docs/ARCHITECTURE.md`, `docs/REQUIREMENTS.md`, and the task-specific docs listed in `AGENTS.md`

## Production

- Current deployed production commit: `bbe19a349f10bfde5f4cf9695c3cf565126eca26`.
- The previously blocking job `108 / UPLOAD_BILIBILI` completed before deployment; the running-job guard was not
  bypassed. Dev CI `35287008538`, main CI `35287392831`, and Production Deploy `35287393082` passed. Public readiness
  reported the deployed SHA, `/admin/jobs` returned the new frontend entry and main asset, and unauthenticated
  `/api/v1/me` continued to return `401 NOT_AUTHENTICATED`.
- `main` runs the reusable CI gate before the release job; `dev` runs CI only.
- The current release passed backend format, tidy, vet, tests, build, clean-database migration smoke, frontend
  lint/typecheck/tests/build, Compose validation, and production deployment.
- CI runs `35161716643` (dev) and `35161858719` (main) passed the complete repository gate. Production Deploy run
  `35161859027` completed successfully, and `https://7g.chat/health/ready` reported the deployed SHA.
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
- Parent discovery does not use possibly stale Profile `LIVE/RECORDING` runtime values as a permanent gate. It waits
  for the merge gap and uses adjacent `ACTIVE` recordings, `WRITING` video rows, and recent adjacent recorder files as
  the authoritative active-recording evidence.
- The worker polls the Recorder room endpoint every 30 seconds to refresh Profile runtime display state. A failed
  runtime read retains the last known state but cannot block closed-file discovery.
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
  ignored by BililiveRecorder. Production verification on 2026-09-13 confirmed `RecordDanmaku=true` in Recorder's
  saved room config and one closed XML file beside each of five closed video segments.
- Raw danmaku files are indexed only when BililiveRecorder actually writes a closed danmaku asset.
- Closed raw danmaku assets are archived byte-for-byte to COS under the controlled `raw/` prefix.
- Parsing, merging, and aligning danmaku to edited/split video timelines remains out of scope pending real samples.

The next local development batch replaces this production behavior with authorized OpenLive capture. The target
disables new Recorder XML writing and new raw-XML COS reconciliation while retaining historical XML metadata/files.
Production still has the behavior above until this batch is explicitly deployed and configured.

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
- The interrupted 2026-09-12 Bilibili job was retried only after Creator Center was checked and no matching submission
  existed; it subsequently completed successfully. Do not edit SQLite statuses manually for future ambiguous jobs.

## Future Design Documents

- Frontend iteration is now the active design direction: retain React/TypeScript/Vite, modularize console features,
  introduce real page routes and shared modern UI, and support expressive public streamer pages with separate layouts
  and lazy-loaded animation. `docs/FRONTEND_UI.md` records the target and delivery checkpoints. Local synthetic-data
  development and isolated integration/testing are required; remote staging location remains undecided and `dev`
  stays CI-only. All existing console pages now use independent feature modules under the shared layout/theme;
  production routing does not import the compatibility AdminDashboard test harness. Public handle matching is corrected.
  See `docs/FRONTEND_DEVELOPMENT.md` for mock, isolated integration and built-preview modes. The pinned pnpm lockfile,
  generated console resource types and Playwright mock/real-backend tests are included. No production deployment or
  database schema change occurred. Verification covers permission gates, draft retention, review/download/edit
  separation, cleanup confirmation, empty-password preservation, and real-backend profile/upload settings persistence.
  Local verification passed: frontend lint/typecheck/build, 18 unit/component tests, 13 mock browser tests,
  2 built-frontend/real-backend integration tests, generated contract comparison, backend full tests and vet.
  The previous local built preview used `http://127.0.0.1:4173/admin` (built frontend + fresh backend);
  synthetic account: `local-admin` / `local-test-only-password`. Data: `data/dev/run-myvshbdk`.
  Fixture demonstration is at `http://127.0.0.1:5173/admin/recordings`, with no login required.
  The built preview was stopped to run the next local integration suite. These are local processes, not persistent remote services. They use no inherited external integration credentials;
  production remains unchanged. Recreate with the commands in `FRONTEND_DEVELOPMENT.md` after stopping them.

- `docs/LIVE_ANALYTICS_RESEARCH.md` records the operations-analytics research, metric limitations, Recorder 2.18.0
  capture gaps, and official Bilibili API options. A normal viewer account is allowed; target-streamer credentials or
  authorization are not assumed. The local OpenLive probe used the user's authorized test-room identity code and
  project `1793018783146`: start, WebSocket auth, API/WS heartbeat, one danmaku event, and end all succeeded while the
  test room was offline. No credential, identity code, event body, collector, schema, production setting, or dependency
  was committed. This proves the unlisted project can connect the developer's authorized test room; it does not prove
  other-room access, complete event permissions, or production approval. Scope remains live-stream data and its
  retrospective analysis only; uploaded-recording video analytics is excluded.
  The dynamic OpenLive documentation can now be read locally via `docs/references/bilibili-open-live/README.md`.
  Its published directory was discovered using an anonymous Playwright/Chrome session; the manual archive script is
  `scripts/docs/archive_bilibili_open_live.py`. Reference snapshots do not replace sanitized runtime fixtures.
- `docs/7GRecorder_Songs_V1_Technical_Design.md` now targets manually selecting one AVAILABLE COS video output, running
  a free local high-recall singing candidate detector, automatically creating permanent COS M4A artifacts with a 5%
  local playback cache, reviewing/labeling worthwhile performances, and creating accurate MP4 exports on demand with
  a 5GB local cache. Automatic title identification and original-versus-cover classification are not V1 requirements.
- Production `6356f1910d1b0e3ecac856336233053ded963251` contains the minimum end-to-end Songs MVP. A dedicated `AI`
  worker extracts one low-bitrate analysis MP3, streams it to the documented ACRCloud File Scanning API, polls and
  persists the result, creates Song drafts, cuts M4A from the original video, uploads versioned artifacts to COS,
  keeps playback copies under the 5% managed-local cache budget, and lists/plays cached audio in the admin UI.
- Dev and main CI passed on 2026-09-15, including the Linux Worker end-to-end fake-adapter test; production deployment
  and `https://7g.chat/health/ready` both reported the same commit SHA.

Cache-miss COS refill, editable boundaries, multi-chunk analysis, confirmation, and on-demand MP4 export remain
pending. ACRCloud is no longer the target provider because its useful ongoing service is paid after a limited trial.
The next checkpoint must pin and benchmark the local detector, then replace external submission/polling for new Runs;
that replacement is approved design but is not deployed yet. Songs development is now paused while operations features
take priority. When resumed, benchmark CPU-only PANNs MobileNetV2 and Cnn6 first, record actual disk/RAM/runtime costs,
and consider Cnn14 only if both lightweight candidates miss the recall target.

Production `06f27e3e2e0f112bebe4a8b0bd2b3e2589fc11d1` includes PK resolution normalization for future upload-source
packaging. When dimensions and their derived H.264 level are the only stream differences, FFmpeg chooses the existing
resolution with the greatest cumulative duration, keeps each source aspect ratio, avoids enlarging smaller inputs,
centers them on black padding, and returns to the normal two-hour/size-aware part policy. Other stream differences and
normalization failures still use ordered compatibility-boundary parts. Existing Bilibili/COS submissions are not
rewritten automatically.

Do not introduce Redis, RabbitMQ, Kafka, PostgreSQL, or a workflow engine for these items without a new design review.


## Frontend migration rollout

The frontend migration and Job heartbeat/progress visibility fix were deployed on 2026-09-18.
System storage/TLS forms now retain local drafts through polling and background errors, preserve newer edits when an
older save completes, show per-form save feedback, and confirm route departure. Refresh/close uses beforeunload.
Upload Bilibili/COS and song settings now use the same draft handling. Profile switches confirm discarding changes
and cannot change the submission target during a save. Profile/account dialogs support focus containment, Escape
and discard confirmation; account passwords and credential material remain in memory only. Unapplied review cuts
block approval and warn before departure. Mobile account tables scroll within their panel. Background fetch failures
retain loaded module data and edits. COS disabling explicitly leaves other edited fields unsaved, matching the API.
All existing console modules are migrated; the legacy dashboard is retained only for compatibility component tests.
New public creative pages and live analytics remain future feature work.

Local validation for this iteration passed: frontend lint/typecheck/build, 20 unit/component tests,
24 synthetic browser tests and 4 real-backend integration tests. Local fixture and isolated integration processes
were stopped after validation.
The migration is present on both `dev` and `main` and is now the production frontend.

A subsequent local-only fix separates Worker liveness from external-tool progress. Claimed Jobs refresh
`heartbeat_at` every 15 seconds without changing `progress_updated_at`; updates are lock-owner scoped. The Jobs UI now
shows both timestamps and distinguishes a live Job with quiet/unparseable biliup progress from a stale Worker. Local
verification: one focused Worker heartbeat unit test, frontend lint/typecheck/build, 22 unit/component tests, and 24
synthetic browser tests. Per user instruction, no full backend suite or real-backend local environment was run. This
fix is deployed in `bbe19a349f10bfde5f4cf9695c3cf565126eca26`.

## Local development after the current production release

Live operations analytics capture is under local development and is not deployed. The current implementation adds
OpenLive signed start/heartbeat/end calls, WebSocket auth/heartbeat and version 0/zlib packet parsing, encrypted
Profile configuration, capture-session metadata and gap counts, and loss-minimizing JSONL storage for every decoded
CMD including unknown future commands. A real authorized test-room smoke completed start, WebSocket authentication
and end using local environment credentials without logging secrets or payloads.

The console now has a Live Analytics configuration/status page. Recording rows link to a new-tab session detail page
organized into overview, files, interaction trends, danmaku hotspots, gift/SC/guard, and capture-quality sections.
Only capture health and retained-event totals are populated in this checkpoint; metric aggregation waits for real
live samples. Local focused backend tests and migration smoke passed. Frontend lint, typecheck, 22 component tests,
build, the existing 24-browser-test suite, and 8 focused console browser tests passed. No production deployment was
performed.

`live-analytics/` raw JSONL is now included in managed local-storage accounting with a fixed 2 GiB rolling subquota.
The collector checks it every minute, protects active-session files, and deletes the oldest ended-session evidence
until usage returns below the cap. Session totals, the last observed raw size, deletion timestamp, event counts, and
capture gaps remain in SQLite. The System page shows raw/subquota and combined managed usage; session/detail pages
show raw evidence state. Focused backend tests, backend build, frontend lint/typecheck, 22 component tests, frontend
build, and 26 mock browser tests pass. Deployment still requires saving and validating the production Profile's
OpenLive credential before relying on the migration that disables new BililiveRecorder XML capture.

Local operations follow-up (not deployed): retained per-minute CMD counts, authenticated bounded event pagination,
capture-session detail route with accessible chart/table, and OpenLive draft guards. Protocol decompression now has
a shared 16 MiB expansion budget and nesting limit; evidence paths reject symlink parents. Release DB backup uses
SQLite online backup and quick_check instead of copying an active WAL database.
The operator resolved the storage policy: remote upload success must not block local rolling cleanup or live capture.
Active/writing/protected/in-use files remain protected; pending and failed remote deliveries do not gate cleanup.

Validation for this local batch: focused Go tests for recording/upload/liveanalytics/db/httpserver passed; backend build passed. Frontend lint/typecheck/build, 22 component tests, 8 console browser tests and the analytics browser test passed. No full backend environment was started and no deployment/push was performed.

Remaining readiness work: a long-running active JSONL is protected from cleanup and can exceed the 2 GiB target; implement file rotation without breaking the OpenLive connection before treating the raw cap as bounded. Minute counters count received messages, not deduplicated viewers or settled revenue.
