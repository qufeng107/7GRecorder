# Frontend UI Contract

## Language

- The admin console defaults to Chinese.
- The current account controls include a Chinese / English language switch.
- Backend enum/status values may remain machine-readable until dedicated display labels are mapped.

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
- Displayed recording timestamps use `Asia/Shanghai`.
- Each recording row can open a details dialog with profile, streamer, China-time timestamps, duration, local status,
  all indexed files, per-file paths, protect action, and per-file download actions.
- Completed recordings shorter than three minutes are marked as short segments. This is an observation label only;
  it must not merge, delete, or hide files.
- The recordings page shows summary metrics for the current filtered list: visible size, short segment count, and
  protected recording count.

## Jobs

- Jobs are a separate operational page, not mixed into recording profiles.
- The page shows job type, related profile, status, attempts, schedule time, and last error.
- Retry is shown only for failed or cancelled jobs. Cancel is shown only for non-running, non-terminal jobs.
- Jobs reuse the shared search and sort toolbar so later server-side filtering can replace client-side filtering cleanly.

## Account Management

- Super admins can open an account editor from the accounts table.
- Empty password fields must keep the current password unchanged.

## Permission-Gated UI

- Manager-only controls should be hidden or replaced with a read-only state when the current `ManagerPolicy` denies the capability.
- Local scan remains super-admin-only because it reconciles shared server storage.
- Recording protect/download actions follow `can_manage_local_files`.
- Destructive local storage actions must require confirmation and show the cleanup result after completion.
