# Frontend UI Contract

## Language

- The admin console defaults to Chinese.
- The current account controls include a Chinese / English language switch.
- User-facing tables should map backend enum/status values to localized display labels when the mapping is known.

## Table Controls

- Table-heavy admin pages should use the shared toolbar shape for search and sort controls.
- Client-side search and sorting are acceptable while the dataset is small.
- Future server-side pagination, filtering, and column sorting should reuse the same toolbar shape.
- Wide data tables use TanStack Table for table state and column resizing while keeping the existing Tailwind UI.
- Table action buttons should keep a stable width and stay on one line.

## Recording History

- Recording start and completion timestamps are first-class table columns.
- Timestamp headers show the China Time label on a second line. Timestamp cells show date and clock time on separate
  lines without repeating the timezone label.
- Duration appears immediately after completion time in the recordings table.
- Displayed recording timestamps use `Asia/Shanghai`.
- Each recording row can open a details dialog with profile, streamer, China-time timestamps, duration, local status,
  all indexed files, per-file paths, protect action, and per-file download actions.
- Completed source segments shorter than three minutes count toward the short-segment summary. Upload-source rows do
  not show a per-row short-segment badge.
- The recordings page uses upload sources as the primary rows. Each row represents one upload-facing video. Expanding a
  row shows child source segments with their recording timestamps, timeline intervals, sizes, and paths.
- Upload-source downloads are shown only for publish parts with COS status `AVAILABLE`; the UI requests a short-lived
  COS URL from the backend instead of linking to local server files. Parent upload-source rows never show download
  actions, even when there is only one publish part. Verified Bilibili publications are shown as external video links.
  Expanded recording rows show publish parts before original segments because downstream upload actions operate on
  publish parts. Original segments are collapsed by default and can be expanded inside the nested row detail area.
- Publish part rows should show COS and Bilibili status separately. When COS compression is enabled, show source size,
  uploaded object size, compression status, and compression preset/gain in the detail area without making compressed
  COS files look like separate recordings.
- Multi-segment upload sources can derive their temporary "merging" display state from matching `MERGE_UPLOAD_SOURCE`
  jobs while the upload-source row itself is still `MERGE_PENDING`.
- The recordings page shows summary metrics for the current filtered list: visible size, short segment count, and
  protected recording count.
- The recordings table keeps its action column sticky on the right during horizontal scrolling.

## Jobs

- Jobs are a separate operational page, not mixed into recording profiles.
- The page shows job type, related profile, status, attempts, schedule time, and last error.
- The page has an explicit refresh button in addition to automatic polling.
- Retry is shown only for failed or cancelled jobs. Cancel is shown only for non-running, non-terminal jobs.
- Jobs reuse the shared search and sort toolbar so later server-side filtering can replace client-side filtering cleanly.

## Upload Settings

- Bilibili and COS settings are managed from a dedicated Upload Settings page, separate from Recording Profiles and
  system-wide Local Storage settings.
- Upload settings are scoped to a selected Recording Profile.
- Bilibili settings expose editable title template, description template, tags, and copyright fields instead of asking
  users to edit raw JSON for common posting metadata.
- Credentials are created from the Upload Settings page and are displayed as metadata only after creation; plaintext
  secrets must never be shown again.
- Managers can see the Upload Settings page only when their ManagerPolicy allows at least one upload module.
- Creating upload jobs from ready Upload Sources remains super-admin-only.

## Account Management

- Super admins can open an account editor from the accounts table.
- Empty password fields must keep the current password unchanged.

## Permission-Gated UI

- Manager-only controls should be hidden or replaced with a read-only state when the current `ManagerPolicy` denies the capability.
- Local scan remains super-admin-only because it reconciles shared server storage.
- Recording protect/download actions follow `can_manage_local_files`.
- Destructive local storage actions must require confirmation and show the cleanup result after completion.
