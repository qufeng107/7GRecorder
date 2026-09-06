# Frontend UI Contract

## Language

- The admin console defaults to Chinese.
- The current account controls include a Chinese / English language switch.
- Backend enum/status values may remain machine-readable until dedicated display labels are mapped.

## Table Controls

- Table-heavy admin pages should use the shared toolbar shape for search and sort controls.
- Client-side search and sorting are acceptable while the dataset is small.
- Future server-side pagination, filtering, and column sorting should reuse the same toolbar shape.

## Recording History

- Recording start time is a first-class table column.
- Displayed recording timestamps use `Asia/Shanghai` and include the visible China Time label.

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
