# 7GRecorder Songs V1 Technical Design

> Version: V1.2
> Status: approved design, implementation in progress
> Scope: manual recognition of one available COS video output, automatic M4A artifacts, on-demand MP4 exports.

## 1. Goal

Songs is an optional business boundary. A SUPER_ADMIN manually selects one video part already available in Tencent COS
and starts recognition. The system identifies songs and their time ranges, automatically creates an M4A for every
draft, uploads those audio artifacts to COS, and presents them in an admin list.

There are two distinct user actions:

```text
Play
  -> use the automatically generated M4A
  -> use the local audio cache when present
  -> otherwise cache the M4A from COS, then play it

Download song video
  -> do not pre-generate video during recognition
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

## 3. Source Identity And Timeline

Only these objects are selectable:

```text
upload_source_cos_objects.status = AVAILABLE
upload_source_outputs.status = READY_TO_UPLOAD
upload_sources.status = READY_TO_UPLOAD
```

Legacy `cos_objects` rows are excluded because they currently represent raw recording-file attachments, primarily
danmaku files.

A run snapshots the selected object ID/key/ETag/size, output timeline, storage profile, provider settings version, and
algorithm version. Recognition offsets are relative to the selected output. Song boundaries use the parent Upload
Source timeline:

```text
song.start_ms = output.timeline_start_ms + recognized_start_ms
song.end_ms   = output.timeline_start_ms + recognized_end_ms
```

This preserves future multi-output compatibility while keeping V1 single-file.

## 4. Recognition Provider

V1 uses an operator-created ACRCloud File Scanning container configured for Audio Fingerprinting plus Cover Song
Identification, Traverse Scanning, and Music Detection. 7GRecorder polls results and exposes no callback.

Authoritative provider references:

- `https://docs.acrcloud.com/reference/console-api/file-scanning`
- `https://docs.acrcloud.com/reference/console-api/file-scanning/file-scanning`
- `https://docs.acrcloud.com/reference/console-api/file-scanning/metadata/cover-songs`

The production adapter must be based on sanitized responses captured from the configured test container. Documentation
examples alone are not accepted as fixtures, and implementation must not guess unobserved payload fields.

Encrypted credential:

```text
scope     SYSTEM
platform  acrcloud
purpose   SONG_RECOGNITION
secret    {"access_token":"..."}
```

Non-secret settings contain region and container ID. V1 does not create or mutate the external container. Provider
responses are immutable evidence; Song drafts remain editable and reviewable.

## 5. Durable Flow

```text
select AVAILABLE COS output
  -> create song_analysis_run and reserve working space
  -> DOWNLOAD_SONG_SOURCE                  NETWORK
  -> EXTRACT_SONG_ANALYSIS_CHUNKS          MEDIA
  -> SUBMIT_ACRCLOUD_SCAN                  AI
  -> FETCH_ACRCLOUD_RESULT                 AI
  -> FINALIZE_SONG_RECOGNITION             LIGHT
  -> create evidence and Song drafts
  -> CUT_SONG_AUDIO                        MEDIA
  -> UPLOAD_SONG_AUDIO                     NETWORK
  -> show drafts with available audio or a visible failure
```

Analysis audio defaults to MP3, mono, 32 kHz, 64 kbps, two-hour cores, and 30-second guards on both sides. Provider
filenames contain run ID, chunk index, and algorithm version. Before retrying an ambiguous submission, the adapter
searches for that deterministic name and reuses an existing provider file ID to avoid repeated paid scans.

Polling backoff is 30 seconds, one minute, two minutes, then five minutes. Provider IDs and poll state are persisted
business state, not only Job payload data.

## 6. Aggregation And Boundaries

Fingerprint and cover matches normalize to one evidence type. Aggregation converts chunk offsets, deduplicates guard
overlap, clusters by time before resolving identity, prefers ACRID/ISRC over normalized title/artist, and keeps
conflicts as candidates.

```text
detected_start_ms / detected_end_ms  provider-derived boundary
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

- `song_settings`: provider credential, region/container, destination COS profile/prefix, padding and algorithm version.
- `song_analysis_runs`: source/config snapshots, status/progress, reservation, error and timestamps.
- `song_analysis_chunks`: core/guard ranges, provider file identity/state/result and polling state.
- `song_recognition_matches`: immutable normalized evidence and raw provider fixture data.
- `songs`: Upload Source/run ownership, nullable Recording mapping, detected/current ranges, revision and review state.
- `song_artifacts`: versioned authoritative COS M4A metadata, ETag, size, status and replacement relation.
- `media_cache_entries`: disposable local cache identity, size/state, access time and grace/lease.
- `storage_reservations`: durable peak-byte reservations owned and heartbeated by Jobs.

Run statuses:

```text
PENDING | DOWNLOADING | ANALYZING | RECOGNIZING | FINALIZING
| GENERATING_AUDIO | REVIEW_REQUIRED | COMPLETED | FAILED | SOURCE_MISSING | CANCELLED
```

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
- sanitized provider fixtures for processing, ready, no-result, auth, malformed and conflicting results;
- duplicate-scan recovery; guard deduplication; timeline conversion and boundary clamp;
- revision/recut/artifact upload/stale-cache invalidation;
- M4A and accurate MP4 generation from original media;
- playback hit/miss/coalescing and authenticated internal redirect;
- reservation races and LRU eviction with lease/grace protection;
- source COS cleanup lease; restart recovery; optional-module isolation;
- frontend source selection, progress, playback, editing and video-export states.

## 16. Delivery Order

1. Capture sanitized ACRCloud fixtures from an operator-created test container.
2. Update schema and storage-budget semantics.
3. Implement reservation/cache primitives and safety tests.
4. Implement source selection and COS downloader.
5. Implement ACRCloud adapter and durable analysis state machine.
6. Implement aggregation and automatic M4A upload.
7. Implement list/playback/editing UI.
8. Implement on-demand accurate MP4 export/download.
9. Run CI and accept one small production COS video before larger sources.

Current development checkpoint:

- schema, SUPER_ADMIN settings API, encrypted ACRCloud credential entry, AVAILABLE COS source selection, and Run creation are implemented;
- `DOWNLOAD_SONG_SOURCE` streams from COS into `.part`, verifies the snapshotted ETag/size, atomically promotes the file,
  reports progress, and participates in managed-local-space reservation;
- the admin Songs page can configure the module, select a COS source, start a Run, and observe its current state;
- ACRCloud submission/polling remains blocked until sanitized fixtures are captured from the operator-created test
  container; do not enable or deploy Songs to production before that adapter and all subsequent stages are complete.
