# Upload Sources Plan

## Goal

Continuous livestreams can appear as several completed local recordings when the upstream stream reconnects, the
recording adapter rotates files, or the network briefly drops. 7GRecorder groups adjacent recordings from the same
Recording Profile before later upload work.

## Current Scope

- Read-only recording group diagnostics are computed from existing `recordings` and `recording_files` metadata.
- Adjacent recordings from the same profile belong to one upload source when the gap from previous completion to next
  start is less than or equal to the merge gap threshold. The current default is 600 seconds.
- A group is finalized only after the latest source recording has been completed for longer than the same merge gap
  threshold. This keeps "gap grouping" and "wait before finalizing" as one user-facing setting.
- Completed recordings shorter than 180 seconds are marked as short segments.
- Admin UI treats upload sources as the primary recording list. Expanding a row shows pre-package segments, their
  source recording timestamps, and their timeline interval inside the upload source.
- Single-segment upload sources and merged upload sources both pass through `PACKAGE_PENDING`. Packaging records the
  post-package parts that Bilibili and COS consume. Parts are named as
  `<profile-name>-<YYYYMMDD>-第NN场直播-pNN.flv`, where the date and live ordinal use China time for that recording
  profile. If the file is already within limits, packaging creates one named part; larger or longer files are split
  under `DATA_ROOT/upload-sources/<profile-id>/<source-id>/parts/`.
- Multi-segment upload sources are marked `MERGE_PENDING` until an FFmpeg concat job creates the merged file, then
  `PACKAGE_PENDING` until packaging finishes.
- Discovery also backfills missing `MERGE_UPLOAD_SOURCE` jobs for existing `MERGE_PENDING` upload sources so records
  created by older releases do not remain stuck without work.
- Discovery also backfills missing `PACKAGE_UPLOAD_SOURCE` jobs for existing `PACKAGE_PENDING` sources and older
  `READY_TO_UPLOAD` sources that predate durable output parts.

## Upload Merge

`MERGE_UPLOAD_SOURCE` is a MEDIA job created idempotently when a multi-segment upload source is discovered or when
discovery finds an older `MERGE_PENDING` source without its merge job.

Rules:

- preserve every original recording file;
- only use CLOSED local video files inside `DATA_ROOT`;
- reject groups with missing, deleted, writing, or path-unsafe files;
- write derived files under `DATA_ROOT/upload-sources/<profile-id>/<source-id>/`;
- store enough metadata to trace the derived file back to source recording IDs and China-time windows;
- mark the upload source `PACKAGE_PENDING` after the merged output file is written;
- keep failed sources visible as `MERGE_PENDING` while retryable and `MERGE_FAILED` after terminal failure;
- upload modules consume only post-package output parts from upload sources with `READY_TO_UPLOAD` status.

## Upload Packaging

`PACKAGE_UPLOAD_SOURCE` is a MEDIA job. It applies one shared delivery boundary for COS and Bilibili:

- default maximum part size is 4 GiB (`UPLOAD_MAX_PART_BYTES=4294967296`);
- default maximum part duration is 2 hours (`UPLOAD_MAX_PART_DURATION_SECONDS=7200`);
- the size default stays below the current 5GB COS simple upload limit and leaves room for platform/account variation;
- output parts preserve source timeline metadata so later publisher modules can include segment provenance.
- workers run reconciliation on a fixed interval, so local recording indexing, discovery, merge job backfill, package
  job backfill, and upload module job creation do not depend on manually pressing Scan.

## Non-Goals

- Do not switch recorder software until diagnostics show BililiveRecorder is the actual source of loss.
- Do not create ZIP downloads for multi-segment recordings.
- Do not let Bilibili/COS/Songs depend on Recording Core success beyond their own source availability checks.
