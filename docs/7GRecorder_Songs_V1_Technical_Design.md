# 7GRecorder Songs V1 Technical Design

> Version: V1.3
> Status: approved design, implementation in progress
> Scope: manual analysis of one available COS video output, local high-recall singing candidate detection, automatic
> M4A artifacts, human curation, and on-demand MP4 exports.

## 1. Goal

Songs is an optional business boundary. A SUPER_ADMIN manually selects one video part already available in Tencent COS
and starts analysis. The system uses a free local detector to find time ranges likely to contain singing, automatically
creates an M4A for every candidate draft, uploads those audio artifacts to COS, and presents them in an efficient admin
review list.

The product objective is to reduce the time spent scrubbing full recordings. Automatic title identification and the
distinction between an original recording and a live cover are not required: a human reviewer chooses worthwhile
performances, adjusts their boundaries, and supplies final metadata.

There are two distinct user actions:

```text
Play
  -> use the automatically generated M4A
  -> use the local audio cache when present
  -> otherwise cache the M4A from COS, then play it

Download song video
  -> do not pre-generate video during analysis
  -> create an export job on demand
  -> obtain the original COS video source
  -> cut an accurate MP4 for the current boundary revision
  -> cache the MP4 locally and offer an authenticated download
```

## 2. Non-goals

V1 does not automatically scan recordings, use Local Source, analyze multiple COS outputs in one run, merge across
COS output boundaries, publish to music platforms, pre-generate video exports, or keep video exports in COS. Songs
failure never changes Recording, Bilibili, or source COS state. Browsers never receive COS credentials, permanent COS
URLs, or local paths.

V1 does not promise automatic song titles, artists, original-versus-cover classification, a global reference catalog,
lyrics matching, or zero false positives. High recall is preferred over high precision because rejected candidates are
cheap to review while a missed high-quality performance may require scrubbing the full recording again.

## 3. Source Identity And Timeline

Only these objects are selectable:

```text
upload_source_cos_objects.status = AVAILABLE
upload_source_outputs.status = READY_TO_UPLOAD
upload_sources.status = READY_TO_UPLOAD
```

Legacy `cos_objects` rows are excluded because they currently represent raw recording-file attachments, primarily
danmaku files.

A run snapshots the selected object ID/key/ETag/size, output timeline, storage profile, detector settings/model
version, and algorithm version. Detection offsets are relative to the selected output. Song boundaries use the parent Upload
Source timeline:

```text
song.start_ms = output.timeline_start_ms + detected_local_start_ms
song.end_ms   = output.timeline_start_ms + detected_local_end_ms
```

This preserves future multi-output compatibility while keeping V1 single-file.

## 4. Local Candidate Detector

V1 uses a local `SingingCandidateDetector` adapter. It accepts a controlled analysis-audio path plus window settings
and returns timestamped scores for singing, music, and speech. It does not return or infer a song identity.

The first implementation target is a pinned lightweight PANNs AudioSet-compatible checkpoint executed locally as a
batch tool, not as another always-on service. The offline benchmark compares MobileNetV2 and Cnn6 first; full Cnn14 is
considered only when both lightweight models miss the acceptance target. Production uses CPU-only inference, batch
size one, and does not install CUDA dependencies. Before production adoption, the exact code version, model artifact,
checksum, license, runtime dependencies, labels, and normalized output schema must be fixed in `INTEGRATIONS.md`. A
repository fixture set must include representative Chinese live-stream speech, singing, applause, silence, and
speech-over-background-music.

The acceptance gate is operational rather than benchmark-only: on a manually annotated local sample set, the detector
must save meaningful review time and must not frequently omit complete high-quality singing sections. If the first
model fails that gate, the adapter contract remains stable while the implementation can be replaced.

The detector requires no API credential and incurs no per-file external-service fee. CPU, memory, disk, and processing
time remain real local costs. New AI work does not start while recording is active and remains subordinate to Resource
Guard and storage reservations.

Model weights and the pinned CPU runtime are immutable application assets, not disposable Songs media and not part of
the 5% audio playback cache. They still consume system disk and must be included in deploy-time free-space checks and
release cleanup accounting. The selected lightweight checkpoint should be roughly 150 MB; the complete CPU inference
environment is measured during packaging and recorded before deployment. Analysis audio is chunked and deleted after
the owning Run stage so it cannot accumulate as an unbounded second media library.

The existing ACRCloud adapter and encrypted credentials are a deployed legacy checkpoint, not the V1 target. They stay
disabled and are not required for new Runs. Removal of obsolete provider settings and evidence columns is deferred
until the local path is accepted, so rollback does not require a destructive migration.

## 5. Durable Flow

```text
select AVAILABLE COS output
  -> create song_analysis_run and reserve working space
  -> DOWNLOAD_SONG_SOURCE                  NETWORK
  -> EXTRACT_SONG_ANALYSIS_AUDIO           MEDIA
  -> DETECT_SINGING_CANDIDATES             AI
  -> AGGREGATE_SINGING_INTERVALS            LIGHT
  -> create scored interval evidence and untitled Song drafts
  -> CUT_SONG_AUDIO                        MEDIA
  -> UPLOAD_SONG_AUDIO                     NETWORK
  -> show drafts with available audio or a visible failure
```

Analysis audio is mono 32 kHz and is divided into deterministic local chunks when necessary. The initial detector
defaults are 10-second windows with a 5-second hop. Model inference may be retried from persisted chunk state without
redownloading the COS source when the verified working file still exists. There is no external submission or polling.

## 6. Aggregation And Boundaries

Each analysis window produces immutable scores and detector metadata. Aggregation applies configurable thresholds and
hysteresis, merges overlapping/adjacent positive windows, bridges short gaps, filters implausibly short ranges, and
adds bounded lead/tail padding. Initial tuning targets are:

```text
window                     10 seconds
hop                         5 seconds
minimum candidate          30 seconds
merge gap                  20 seconds
lead padding               10 seconds
tail padding               15 seconds
```

Thresholds are calibrated from local fixtures and stored with `algorithm_version`; the values above are starting
points, not silent universal constants. Detection evidence never supplies title or artist. A draft remains useful with
both fields null until human review.

```text
detected_start_ms / detected_end_ms  detector-derived boundary
start_ms / end_ms                    current editable boundary
clip_revision                        incremented after a boundary change
```

Padding is configurable and clamped to the selected output. V1 never aggregates across its boundary.

## 7. Automatic Audio Artifact

Every draft is cut from the downloaded original video, never from analysis MP3:

```text
M4A, AAC 192 kbps, preserve source channels up to stereo
<songs-prefix>/<profile-id>/<song-id>/audio-r<clip-revision>.m4a
```

The COS M4A is authoritative. Its local copy is disposable playback cache. Editing boundaries increments
`clip_revision`, uploads a new M4A, and invalidates old audio/video caches. It does not generate a new video export.

## 8. Playback

`POST /api/v1/songs/{id}/actions/prepare-playback` returns a ready playback URL on a cache hit. On a miss it creates one
deterministic `CACHE_SONG_AUDIO` Job and returns `202` with the Job ID; concurrent misses coalesce. The UI polls and
starts playback when ready.

`GET /api/v1/songs/{id}/audio` is authenticated and uses Nginx `X-Accel-Redirect`. It touches `last_accessed_at` and
adds a short grace period before returning, preventing eviction from racing Nginx opening the file.

## 9. On-demand Video Export

`POST /api/v1/songs/{id}/actions/export-video` uses business key:

```text
song:<song-id>:video-export:<clip-revision>
```

The worker downloads or reuses the original source and produces an accurate MP4 with H.264 video and AAC audio while
preserving size/FPS where practical. V1 re-encodes because stream copy moves cuts to nearby keyframes. Production video
export concurrency is one. The MP4 is local disposable cache, is not uploaded to COS, and is reused for repeated
requests of the same revision.

## 10. Local Storage Budget

The existing `max_recording_bytes` value becomes the backward-compatible source for this broader concept:

```text
max_managed_local_bytes = all local bytes managed by 7GRecorder
```

The API may retain the old field temporarily, but UI/domain wording becomes "7GRecorder local storage limit". Managed
usage includes recordings, upload-source derivatives, Songs caches, and Songs working files. It excludes SQLite,
logs, and externally managed files.

```text
audio playback cache    5% of max_managed_local_bytes
retained video exports  min(5 GB, available global managed budget)
Songs working files     reserved per Job and deleted after the owning stage
```

The 5GB limit applies to retained generated MP4 files, not a complete source video. A source is a short-lived working
file and may exceed 5GB only when a global reservation and free-space guards allow it. This supports historical COS
objects larger than 5GB.

Heavy Jobs transactionally reserve estimated peak bytes. Reservations expire unless heartbeated. When space is not
available, the Job returns to `PENDING` with a later `run_after` and visible `WAITING_FOR_SPACE`; it does not occupy a
worker slot while sleeping.

Reclaim order:

```text
stale Songs work files
-> oldest unleased cache in the requested class
-> oldest unleased cache in the other Songs class
-> recordings eligible under existing safety rules
-> defer when no safe candidate exists
```

Eviction never deletes protected/current recordings, active files, running-Job inputs, leased entries, or recently
served playback files. Lowering a limit schedules reconciliation rather than deleting synchronously.

## 11. COS Lease And Retention

Active analysis/export holds a durable lease on its source COS object, so managed COS cleanup skips it. This is a
temporary deletion guard, not a success dependency. Externally deleted sources produce `SOURCE_MISSING`; existing Song
metadata and M4A artifacts remain.

Song audio artifacts have registered object rows and are not inferred or deleted by upload-source COS cleanup.

## 12. Data Model

- `song_settings`: destination COS profile/prefix, detector thresholds/window/merge/padding settings and algorithm
  version. Legacy provider fields remain nullable during migration.
- `song_analysis_runs`: source/config/model snapshots, status/progress, reservation, error and timestamps.
- `song_analysis_chunks`: local chunk ranges, analysis state, detector output and retry state. Legacy provider identity
  fields remain unused by local Runs.
- `song_recognition_matches`: immutable per-window or aggregated detector evidence. Existing rows from the legacy
  ACRCloud checkpoint remain readable.
- `songs`: Upload Source/run ownership, nullable Recording mapping, detected/current ranges, revision and review state.
- `song_artifacts`: versioned authoritative COS M4A metadata, ETag, size, status and replacement relation.
- `media_cache_entries`: disposable local cache identity, size/state, access time and grace/lease.
- `storage_reservations`: durable peak-byte reservations owned and heartbeated by Jobs.

Run statuses:

```text
PENDING | DOWNLOADING | ANALYZING | DETECTING | FINALIZING
| GENERATING_AUDIO | REVIEW_REQUIRED | COMPLETED | FAILED | SOURCE_MISSING | CANCELLED
```

Persisted legacy ACRCloud Runs may retain `RECOGNIZING`; read models and recovery code keep that value readable while
new local Runs use `DETECTING`.

`songs.recording_id` becomes nullable and is filled only when the entire interval maps unambiguously to one Recording.

## 13. API

```text
GET/PUT  /api/v1/song-settings
GET      /api/v1/song-analysis/sources
POST/GET /api/v1/song-analysis/runs
GET      /api/v1/song-analysis/runs/{id}
POST     /api/v1/song-analysis/runs/{id}/actions/retry
POST     /api/v1/song-analysis/runs/{id}/actions/cancel

GET      /api/v1/songs
GET      /api/v1/songs/{id}
PATCH    /api/v1/songs/{id}
POST     /api/v1/songs/{id}/actions/confirm
POST     /api/v1/songs/{id}/actions/reject
POST     /api/v1/songs/{id}/actions/recut-audio
POST     /api/v1/songs/{id}/actions/prepare-playback
GET      /api/v1/songs/{id}/audio
POST     /api/v1/songs/{id}/actions/export-video
GET      /api/v1/song-video-exports/{id}/download
```

V1 mutations are SUPER_ADMIN-only. Broader read/play/download access requires a later explicit Manager policy.

## 14. Safety And Idempotency

- Downloads write `.part`, verify expected size/ETag where available, then atomically rename.
- Files/objects are not `AVAILABLE` before verification.
- Each stage has a deterministic business key and is restart-retryable.
- Cancellation is cooperative for HTTP/FFmpeg and releases leases/reservations in a finalizer.
- Late completion updates only rows still owned by that running attempt.
- Evidence and logs never retain credentials or signed COS URLs.
- Songs failures never change Recording, Bilibili, or source COS states.

## 15. Required Tests

- settings/permission/source eligibility;
- COS streaming download, cancellation, size/ETag verification and atomic promotion;
- pinned local detector command/output fixtures, malformed output, cancellation, timeout and model-version mismatch;
- window/chunk overlap deduplication, threshold hysteresis, short-gap merge, minimum duration, padding, timeline
  conversion and boundary clamp;
- annotated speech/singing/music fixtures measuring candidate recall and review-time usefulness;
- revision/recut/artifact upload/stale-cache invalidation;
- M4A and accurate MP4 generation from original media;
- playback hit/miss/coalescing and authenticated internal redirect;
- reservation races and LRU eviction with lease/grace protection;
- source COS cleanup lease; restart recovery; optional-module isolation;
- frontend source selection, progress, playback, editing and video-export states.

## 16. Delivery Order

Implementation of this section is intentionally deferred while operations features are developed. Resume only through
the benchmark-first sequence below; do not install a model or change production Run behavior merely because this design
is approved.

1. Pin and benchmark CPU-only PANNs MobileNetV2 and Cnn6 on manually annotated short live-stream samples without
   modifying production behavior; capture normalized fixtures and measured disk/RAM/runtime costs.
2. Change settings and Run creation so local detection is the default and requires no external credential.
3. Implement deterministic analysis windows/chunks and durable local detector execution.
4. Implement high-recall interval aggregation and create untitled Song drafts.
5. Reuse the existing automatic M4A/COS/list/playback path for those drafts.
6. Implement fast review actions: play, confirm, reject, edit title/artist and adjust boundaries.
7. Validate one small production COS source, tune thresholds, and record review-time/recall observations.
8. Implement cache-miss COS playback refill and revision-aware recut.
9. Implement on-demand accurate MP4 export/download.

Current deployed checkpoint (`6356f1910d1b0e3ecac856336233053ded963251`, 2026-09-15):

- schema, SUPER_ADMIN settings API, encrypted ACRCloud credential entry, AVAILABLE COS source selection, and Run creation are implemented;
- `DOWNLOAD_SONG_SOURCE` streams from COS into `.part`, verifies the snapshotted ETag/size, atomically promotes the file,
  reports progress, and participates in managed-local-space reservation;
- the admin Songs page can configure the module, select a COS source, start a Run, and observe its current state;
- the single-file MVP is deployed through ACRCloud submission/polling, durable provider state and evidence,
  Song draft creation, automatic M4A generation/upload, list, and cache-hit playback;
- dev/main CI and the production health check passed for this revision;
- the previously planned ACRCloud production-acceptance run was not completed and is no longer required;
- cache-miss playback refill from COS is not part of this checkpoint, so a locally evicted audio artifact remains
  authoritative in COS but is not playable until the refill endpoint is implemented;
- editable boundaries, multi-chunk analysis, and on-demand MP4 export remain subsequent V1 checkpoints.

Design decision after this checkpoint:

- ACRCloud is not acceptable as the required production path because its useful File Scanning capability is a paid
  external service after a limited trial;
- automatic identity and original-versus-cover classification are removed from V1 success criteria;
- the next implementation checkpoint replaces external recognition with free local high-recall singing candidate
  detection while preserving source download, M4A artifacts, playback, review, caching, and video export semantics;
- this local detector path is approved design but is not yet deployed.
- Songs local-detector development is paused while operations work takes priority; no model/runtime should be added to
  production until that work is explicitly resumed.
