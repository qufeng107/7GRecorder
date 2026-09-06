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
- The first implementation is read-only: it never merges, moves, deletes, uploads, or rewrites recording files.
- Admin UI treats upload sources as the primary recording list. Expanding a row shows the original segments, their
  source recording timestamps, and their timeline interval inside the upload source.
- Single-segment upload sources are marked `READY_TO_UPLOAD` immediately. Multi-segment upload sources are marked
  `MERGE_PENDING` until a later FFmpeg concat job creates the derived file.

## Later Upload Merge

The next step is a controlled FFmpeg concat job that creates a derived file for each multi-segment upload source.

Rules:

- preserve every original recording file;
- only use CLOSED local video files inside `DATA_ROOT`;
- reject groups with missing, deleted, writing, or path-unsafe files;
- write derived files under a managed temp/output directory;
- store enough metadata to trace the derived file back to source recording IDs and China-time windows;
- upload modules consume only upload sources with `READY_TO_UPLOAD` status.

## Non-Goals

- Do not switch recorder software until diagnostics show BililiveRecorder is the actual source of loss.
- Do not create ZIP downloads for multi-segment recordings.
- Do not let Bilibili/COS/Songs depend on Recording Core success beyond their own source availability checks.
