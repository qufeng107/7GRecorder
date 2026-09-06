# Recording Groups Plan

## Goal

Continuous livestreams can appear as several completed local recordings when the upstream stream reconnects, the
recording adapter rotates files, or the network briefly drops. 7GRecorder groups adjacent recordings from the same
Recording Profile before later upload work.

## Current Scope

- A group is computed from existing `recordings` and `recording_files` metadata.
- Adjacent recordings from the same profile belong to one group when the gap from previous completion to next start is
  less than or equal to 120 seconds.
- Completed recordings shorter than 180 seconds are marked as short segments.
- The first implementation is read-only: it never merges, moves, deletes, uploads, or rewrites recording files.
- Admin UI shows merge-ready groups with time window, segment count, total duration, total size, max gap, and short
  segment signal.

## Later Upload Merge

The next step is a controlled FFmpeg concat job that creates a derived upload source for each merge-ready group.

Rules:

- preserve every original recording file;
- only use CLOSED local video files inside `DATA_ROOT`;
- reject groups with missing, deleted, writing, or path-unsafe files;
- write derived files under a managed temp/output directory;
- store enough metadata to trace the derived file back to source recording IDs and China-time windows;
- upload modules consume the derived source only after the merge job succeeds.

## Non-Goals

- Do not switch recorder software until diagnostics show BililiveRecorder is the actual source of loss.
- Do not create ZIP downloads for multi-segment recordings.
- Do not let Bilibili/COS/Songs depend on Recording Core success beyond their own source availability checks.
