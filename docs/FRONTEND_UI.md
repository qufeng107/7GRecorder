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

## Upload Review And Editing

- Active recordings appear in the recording list before grouping/package completion so review can be requested early.
- A parent row exposes `Require review` when it is eligible and `Approve review` while its upload source is
  `REQUIRED`. Approval is a separate deliberate action; applying an edit never approves automatically.
- Expanded reviewed rows show local publish-part download actions, a deletion-range editor, edit job state, and the
  independent Bilibili/COS states.
- The range editor accepts one deletion interval per line in `HH:MM:SS-HH:MM:SS` form. Values refer to the complete
  parent timeline, not to the currently displayed part.
- `Apply edit` queues background media work. The UI must keep showing the review block and must not imply success merely
  because the textarea was accepted or cleared.
- The reliable edit-success indicators are: output paths change to `edited/...`, total/output durations and sizes are
  refreshed, the edit job succeeds, and no pending edit decision remains.
- `Approve review` remains blocked by the backend while edit work is pending. After approval, local review downloads
  disappear and Bilibili/COS progress resumes independently.
- Original segments are inspection/audit material and remain visually separate from current publish parts. Upload
  status is always attached to the current publish-part rows.
- For `READY_TO_UPLOAD` sources, the parent status is derived from enabled destination states: `Upload failed` wins over
  `Uploading`, and all enabled destinations successful displays `Upload complete`. `Ready to upload` is reserved for a
  source that still has pending/waiting delivery work.
- Once review is approved, the action label is `Re-review`, not `Require review`. A completed Bilibili publication does
  not show a disabled review button that looks like current state.
- The expanded detail shows the latest Bilibili publication error and per-output COS error when a module is failed.

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


## Modular Frontend Iteration — 2026-09-17

Status: all existing console routes use feature modules and the shared layout; local/test environments implemented. Retain React, TypeScript, Vite,
React Router, TanStack Query/Table and Tailwind. Do not upgrade pinned dependencies as part of structural extraction.
The user wants a modular console, a modern shared UI, local development, and an isolated test environment, with
room for expressive public streamer pages and additional platform modules.

### Application boundaries

- Keep one frontend project initially, with separate console and public layouts and route bundles.
- Preserve `/admin` as the console entry. Give each existing page a real nested URL, such as
  `/admin/recordings`, `/admin/uploads`, `/admin/songs`, and `/admin/jobs`; retain role/policy checks.
  A platform user's dashboard uses their authorized resources, while super-admin operations remain restricted.
  This does not introduce self-registration, new roles, or a new backend permission model.
- Keep `/@:slug/*` for public streamer pages. Public views consume public DTOs only and must not mount
  authenticated console queries, menus, or credential forms.
- Each route owns its loading, empty, error, and permission states. Load substantial feature code on demand;
  public animation dependencies must not be imported by the common console entry.
- Persist shareable filter/sort/page state in URL search parameters. Keep unsaved form state local and handle
  navigation away from modified forms deliberately.

### Module structure and data flow

Use `app/` for routing/providers/layouts, `features/` for business modules, and `shared/` for reusable UI,
API transport, generated contracts, formatting and localization. Create directories only when extracting real code.
Recording profiles, recordings/review, uploads, songs, jobs, accounts, and system settings own their page components,
query hooks and forms. A feature must not import another feature's page or private state.

TanStack Query owns server data. Route-level query activation should avoid polling unrelated hidden pages;
shared health/session queries are explicit exceptions. Query keys and invalidation are defined within each feature.
Logout clears account-scoped cache. The common API client handles structured errors and the existing session/CSRF
contract; it does not duplicate backend authorization. Implement the typed-contract workflow in API_DESIGN.md
instead of moving hand-written duplicate DTOs into a new folder unchanged.

### Shared UI and creative freedom

- Build the console's shared controls using Tailwind and the existing shadcn/ui design direction: buttons, fields,
  dialogs, tables, status labels, loading/empty/error feedback and notifications.
- Define semantic color, typography, spacing, radius and motion tokens. Support coherent light/dark themes and
  keyboard/focus behavior. Extract Chinese/English copy from page implementation.
- The console favors legibility, navigation and efficient repeated operations; public streamer pages may have
  independent typography, composition, backgrounds and richer animation while sharing accessible primitives.
- Begin with CSS for simple transitions. Motion for React is a candidate for public-page gestures, layout and scroll
  animation; add it only with a concrete page requirement and a reviewed pinned version. Consider a 3D/canvas engine
  only for an approved scene rather than including it in the baseline.
- Respect reduced-motion preferences, support touch devices, and offer static fallbacks when animation is disabled
  or a device cannot render the effect reliably. Verify public-page loading and animation separately from console use.
- Public-page SEO, per-streamer link previews and first-render HTML require a separate rendering decision before
  public launch. SPA animation support alone does not satisfy those requirements. Evaluate static pre-rendering first;
  SSR or a new production Node service needs an explicit deployment design if justified.

### Local development and isolated validation

The baseline has three implemented local modes (commands and limitations in `FRONTEND_DEVELOPMENT.md`):

1. Frontend-only local mode with synthetic API fixtures for roles, populated/empty/error/loading states. No production
   API access is needed for UI development; fixture handlers must not silently pass unmatched requests to production.
2. Local integration mode using Vite and a separate local Go backend/SQLite/data root. Use synthetic accounts and
   test files, no production credentials or production recordings; external integrations are disabled or faked.
3. Isolated test environment using the built frontend and test backend with independent database, keys, files, ports
   and cookies. Default to a local reproducible environment. A remotely accessible staging host/domain is a later
   deployment choice, not created automatically by a push to `dev`.

Add Vitest/Testing Library tests per feature and Playwright browser smoke for navigation, refresh/deep links,
permissions, review/edit approval, cleanup confirmation and download actions using fixtures/fake integrations.
UI fixtures aid development; they do not replace API contract checks or real local integration tests.
Record the Node/pnpm versions and commit a lockfile, then use frozen installs in CI. Existing CI checks remain gates.
Browser setup and dependencies must use selected fixed versions. Storybook can be evaluated once shared controls
need independent review; it is not required to begin extracting modules.

### Delivery checkpoints

1. Reproducible local toolchain, lockfile, fixture mode, isolated local integration configuration and browser smoke.
2. Shared API/contracts, layouts, real page routes and one migrated vertical slice (Jobs is the initial candidate).
3. Shared visual primitives/theme and incremental migration of the remaining console modules. Preserve the current
   high-risk confirmations, permission gates, source distinctions and recording timestamp semantics.
4. Public streamer visual prototype with synthetic content, lazy-loaded animation and accessibility/performance checks.
5. Public content integration and a rendering/SEO decision; add future analytics only when its paused design resumes.

Each checkpoint should remain runnable and reviewable. No all-at-once rewrite, automatic production deployment,
production data copy, or new database schema is authorized by this frontend design alone.

### Official references checked 2026-09-17

These establish capabilities, not a dependency upgrade instruction. Check selected package versions before coding.

- React lazy loading: https://react.dev/reference/react/lazy
- React Router modes: https://reactrouter.com/start/modes
- Motion for React: https://motion.dev/docs/react
- Playwright API mocking: https://playwright.dev/docs/mock

### Console migration implementation boundary

All existing console routes use the authenticated layout and lazy feature modules. Each page owns its queries,
mutations and view state; unrelated modules do not mount in the background. Resource contracts are generated from
the backend JSON structs. Shared copy, formatting and fields use the scoped console theme.

The previous AdminDashboard remains only as a compatibility harness for existing component tests; production routes
do not import it. Browser coverage includes profile CRUD/draft retention, review/download/edit separation, account
password preservation, cleanup confirmation, permission gates and mobile layout. Real-backend tests exercise the
built application against a fresh database. The isolated preview is local, not a remote staging deployment.

Recording/account editor drafts survive background refresh. Failed review/protection/download requests produce an
error message without changing the displayed resource state. No new business pipeline or database schema is introduced.
Further visual refinement and public creative content remain later checkpoints.

For this batch, local isolated preview is the default test deployment. A remote host/domain or production release
requires the user's destination choice; do not infer that test deployment authorizes production data access.

### Settings draft protection

System storage/TLS, Bilibili/COS and song settings use an in-memory draft separate from server state.
Polling refreshes untouched fields only when there is no local draft. Saving submits a snapshot; success updates
the query cache and clears only that exact submitted draft, preserving edits made while the request was pending.
Failed saves retain the draft. Each form shows unsaved/saved feedback and an explicit discard action.
Discard adopts the latest fetched server state. Route navigation requires a discard confirmation; reload/closing
uses the browser's native beforeunload warning. Session expiry/logout still clears authenticated state and drafts.
Credential secrets remain memory-only; no localStorage/sessionStorage draft persistence is introduced.
The guard also covers typed TLS credential material. No endpoint, payload, database or deployment behavior changes.
This iteration is local-only at the user's request; do not rerun production deployment.

The completion batch extends draft protection to upload module settings, song settings, profile/account editors,
credential creation and review cut inputs. Bilibili and COS save/discard independently; changing the selected profile
confirms discarding drafts and is disallowed during a save. Snapshots include the target profile ID. Create/editor
forms disable their controls while submitting; close actions confirm discarding changed fields. Background fetch
failures retain previously loaded pages and drafts. Existing review, cleanup, role and empty-password rules remain.
This completes migration of the existing console, not development of new public creative pages or live analytics.

COS disabling preserves the existing API contract: only the enabled state is saved. Other edited fields stay dirty
until explicitly discarded or saved with COS enabled; the UI must not report those fields as saved.
