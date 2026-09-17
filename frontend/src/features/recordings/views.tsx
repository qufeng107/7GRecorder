import {
  type JobItem,
  type AdminCopy,
  type UploadSourceRepairResult,
  type RecordingRegroupResult,
  type RecordingScanResult,
  type RecordingItem,
  type RecordingSortKey,
  type RecordingFile,
  type UploadSourceOutput,
  type UploadSourceSegment,
} from "../../shared/console/types";
import { Fragment, useState } from "react";
import {
  type ColumnDef,
  type ColumnSizingState,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table";
import {
  totalRecordingBytes,
  hasShortSegment,
  currentUploadSourceMergeJob,
  currentUploadSourcePackageJob,
  deriveUploadSourceDisplayStatus,
  formatUploadSourceStatus,
  parseEditCutDraft,
} from "./model";
import {
  formatChinaDateParts,
  formatDuration,
  formatModuleUploadStatus,
  formatBytes,
  formatTimeline,
  formatCompressionStatus,
} from "../../shared/console/format";
import {
  ChevronDown,
  ChevronRight,
  Download,
  FileVideo,
  Lock,
  RefreshCw,
  Save,
  Unlock,
} from "lucide-react";
import {
  TableTimeHeader,
  TableDateTime,
  Metric,
  TableToolbar,
} from "../../shared/console/fields";

export function RecordingsPanel(props: {
  canManageLocalFiles: boolean;
  canScanLocalFiles: boolean;
  cosDownloadPending: boolean;
  cosDownloadPendingOutputId: number | null;
  cosFileDownloadPending: boolean;
  cosFileDownloadPendingFileId: number | null;
  isLoading: boolean;
  jobs: JobItem[];
  labels: AdminCopy;
  protectPending: boolean;
  reviewPending: boolean;
  editDrafts: Record<number, string>;
  editError: boolean;
  editPending: boolean;
  repairError: boolean;
  repairPending: boolean;
  repairResult?: UploadSourceRepairResult;
  regroupError: boolean;
  regroupPending: boolean;
  regroupResult?: RecordingRegroupResult;
  reconcileError: boolean;
  reconcilePending: boolean;
  reconcileResult?: RecordingScanResult;
  recordings: RecordingItem[];
  search: string;
  sort: RecordingSortKey;
  total: number;
  visibleTotal: number;
  onApplyEdit: (uploadSourceId: number, draft: string) => void;
  onApproveReview: (recording: RecordingItem) => void;
  onDownloadFile: (fileId: number) => void;
  onDownloadLocalOutput: (uploadSourceId: number, outputId: number) => void;
  onDownloadOutput: (uploadSourceId: number, outputId: number) => void;
  onEditDraftChange: (uploadSourceId: number, value: string) => void;
  onReconcile: () => void;
  onRegroupToday: () => void;
  onRepairUploadSources: () => void;
  onRequireReview: (recording: RecordingItem) => void;
  onSearchChange: (value: string) => void;
  onSortChange: (value: RecordingSortKey) => void;
  onToggleProtect: (recording: RecordingItem) => void;
}) {
  const [expandedSourceId, setExpandedSourceId] = useState<number | null>(null);
  const [expandedSegmentSourceId, setExpandedSegmentSourceId] = useState<
    number | null
  >(null);
  const [columnSizing, setColumnSizing] = useState<ColumnSizingState>({});
  const toggleSource = (sourceId: number | undefined) => {
    const nextSourceId =
      sourceId && expandedSourceId !== sourceId ? sourceId : null;
    setExpandedSourceId(nextSourceId);
    setExpandedSegmentSourceId(null);
  };
  const visibleSizeBytes = props.recordings.reduce((total, recording) => {
    return total + totalRecordingBytes(recording);
  }, 0);
  const shortSegmentCount = props.recordings.filter((recording) => {
    return hasShortSegment(recording);
  }).length;
  const protectedCount = props.recordings.filter(
    (recording) => recording.local_protected,
  ).length;
  const columns: Array<ColumnDef<RecordingItem>> = [
    {
      id: "recording",
      header: props.labels.recording,
      size: 300,
      minSize: 220,
      cell: ({ row }) => {
        const recording = row.original;
        const file = recording.files?.[0];
        const completedAt = formatChinaDateParts(
          recording.completed_at || file?.closed_at || "",
        );
        return (
          <div className="flex items-start gap-2">
            <FileVideo
              className="mt-0.5 h-4 w-4 shrink-0 text-accent"
              aria-hidden="true"
            />
            <div>
              <button
                className="text-left font-semibold text-ink hover:text-accent"
                type="button"
                onClick={() => toggleSource(recording.upload_source_id)}
              >
                {recording.title ||
                  file?.original_name ||
                  props.labels.untitled}
              </button>
              <p className="mt-1 text-xs text-muted">
                {props.labels.completedAt}: {completedAt.date}{" "}
                {completedAt.time}
              </p>
            </div>
          </div>
        );
      },
    },
    {
      id: "startedAt",
      header: () => (
        <TableTimeHeader title={props.labels.startTime} labels={props.labels} />
      ),
      size: 150,
      minSize: 140,
      cell: ({ row }) => <TableDateTime value={row.original.started_at} />,
    },
    {
      id: "completedAt",
      header: () => (
        <TableTimeHeader
          title={props.labels.completedAt}
          labels={props.labels}
        />
      ),
      size: 150,
      minSize: 140,
      cell: ({ row }) => {
        const file = row.original.files?.[0];
        return (
          <TableDateTime
            value={row.original.completed_at || file?.closed_at || ""}
          />
        );
      },
    },
    {
      id: "duration",
      header: props.labels.duration,
      size: 100,
      minSize: 90,
      cell: ({ row }) => {
        const file = row.original.files?.[0];
        return (
          <span className="text-muted">
            {formatDuration(row.original.duration_ms || file?.duration_ms || 0)}
          </span>
        );
      },
    },
    {
      id: "profile",
      header: props.labels.profile,
      size: 140,
      minSize: 120,
      cell: ({ row }) => (
        <div>
          <p className="font-medium text-ink">{row.original.profile_name}</p>
          <p className="mt-1 text-xs text-muted">{row.original.room_id}</p>
        </div>
      ),
    },
    {
      id: "status",
      header: props.labels.status,
      size: 130,
      minSize: 120,
      cell: ({ row }) => {
        const recording = row.original;
        const file = recording.files?.[0];
        const mergeJob = currentUploadSourceMergeJob(recording, props.jobs);
        const packageJob = currentUploadSourcePackageJob(recording, props.jobs);
        const reviewStatus =
          recording.review_status ?? recording.upload_review_status ?? "NONE";
        const displayStatus =
          reviewStatus === "REQUIRED" || recording.edit_decision_json
            ? "WAITING_REVIEW"
            : deriveUploadSourceDisplayStatus(recording);
        return (
          <div className="text-muted">
            <p>
              {formatUploadSourceStatus(
                displayStatus,
                props.labels,
                mergeJob,
                packageJob,
              )}
            </p>
            {recording.upload_source_id ? (
              <div className="mt-1 space-y-0.5 text-xs">
                <p>
                  {props.labels.bilibiliStatus}:{" "}
                  {formatModuleUploadStatus(
                    recording.bilibili_status,
                    props.labels,
                  )}
                </p>
                <p>
                  {props.labels.cosStatus}:{" "}
                  {formatModuleUploadStatus(recording.cos_status, props.labels)}
                </p>
                {recording.bilibili_status === "FAILED" &&
                recording.bilibili_last_error ? (
                  <p className="break-words text-red-700">
                    {recording.bilibili_last_error}
                  </p>
                ) : null}
                {recording.cos_status === "FAILED" &&
                recording.cos_last_error ? (
                  <p className="break-words text-red-700">
                    {recording.cos_last_error}
                  </p>
                ) : null}
                {recording.local_cleanup_status === "DELETED" ? (
                  <p>{props.labels.uploadSourceLocalCleaned}</p>
                ) : null}
              </div>
            ) : null}
            {recording.upload_source_id ? null : (
              <p className="mt-1 text-xs">
                {file?.file_status ?? props.labels.noFile}
              </p>
            )}
            {reviewStatus === "REQUIRED" ? (
              <p className="mt-1 text-xs font-medium text-amber-700">
                {props.labels.reviewPending}
              </p>
            ) : null}
            {recording.last_error ? (
              <p className="mt-1 break-words text-xs text-red-700">
                {recording.last_error}
              </p>
            ) : null}
            {recording.local_protected ? (
              <p className="mt-1 text-xs font-medium text-accent">
                {props.labels.protected}
              </p>
            ) : null}
          </div>
        );
      },
    },
    {
      id: "size",
      header: props.labels.size,
      size: 90,
      minSize: 80,
      cell: ({ row }) => (
        <span className="text-muted">
          {formatBytes(totalRecordingBytes(row.original))}
        </span>
      ),
    },
    {
      id: "actions",
      header: props.labels.actions,
      size: 140,
      minSize: 132,
      enableResizing: false,
      cell: ({ row }) => {
        const recording = row.original;
        const canUseLocalFile = recording.local_storage_status !== "DELETED";
        const isSingleSegment = (recording.source_segments?.length ?? 0) <= 1;
        const bilibiliURL = (recording.source_outputs ?? []).find(
          (output) => output.bilibili_url,
        )?.bilibili_url;
        const reviewStatus =
          recording.review_status ?? recording.upload_review_status ?? "NONE";
        const deliveryComplete =
          deriveUploadSourceDisplayStatus(recording) === "UPLOAD_COMPLETE";
        if (!props.canManageLocalFiles) {
          return (
            <span className="text-xs text-muted">{props.labels.noAction}</span>
          );
        }
        return (
          <div className="flex flex-col items-start gap-2">
            {recording.upload_source_id ? (
              <button
                className="inline-flex h-8 w-28 items-center justify-center whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                type="button"
                onClick={() => toggleSource(recording.upload_source_id)}
              >
                {props.labels.details}
              </button>
            ) : null}
            {reviewStatus === "REQUIRED" && recording.upload_source_id ? (
              <button
                className="inline-flex h-8 w-28 items-center justify-center whitespace-nowrap rounded-md border border-amber-300 px-3 text-xs font-medium text-amber-800 hover:border-accent hover:text-accent disabled:opacity-60"
                disabled={props.reviewPending}
                type="button"
                onClick={() => props.onApproveReview(recording)}
              >
                {props.labels.approveReview}
              </button>
            ) : deliveryComplete || Boolean(bilibiliURL) ? null : (
              <button
                className="inline-flex h-8 w-28 items-center justify-center whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                disabled={
                  props.reviewPending ||
                  reviewStatus === "REQUIRED" ||
                  Boolean(bilibiliURL)
                }
                type="button"
                onClick={() => props.onRequireReview(recording)}
              >
                {reviewStatus === "APPROVED"
                  ? props.labels.rerequireReview
                  : props.labels.requireReview}
              </button>
            )}
            {canUseLocalFile && isSingleSegment ? (
              <button
                className="inline-flex h-8 w-28 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                disabled={props.protectPending}
                type="button"
                onClick={() => props.onToggleProtect(recording)}
              >
                {recording.local_protected ? (
                  <Unlock className="h-3.5 w-3.5" aria-hidden="true" />
                ) : (
                  <Lock className="h-3.5 w-3.5" aria-hidden="true" />
                )}
                {recording.local_protected
                  ? props.labels.unprotect
                  : props.labels.protect}
              </button>
            ) : null}
            {bilibiliURL ? (
              <a
                className="inline-flex h-8 w-28 items-center justify-center whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                href={bilibiliURL}
                rel="noreferrer"
                target="_blank"
              >
                {props.labels.openBilibili}
              </a>
            ) : null}
          </div>
        );
      },
    },
  ];
  const table = useReactTable({
    data: props.recordings,
    columns,
    columnResizeMode: "onChange",
    defaultColumn: {
      minSize: 80,
      size: 140,
      maxSize: 640,
    },
    state: {
      columnSizing,
    },
    onColumnSizingChange: setColumnSizing,
    getCoreRowModel: getCoreRowModel(),
  });

  return (
    <section
      id="recordings"
      className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">
            {props.labels.uploadSources}
          </h2>
          <p className="mt-1 text-sm text-muted">
            {props.labels.total(props.visibleTotal)} / {props.total}
          </p>
        </div>
        {props.canScanLocalFiles ? (
          <div className="flex flex-wrap items-center gap-2">
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-semibold text-ink hover:border-accent hover:text-accent disabled:opacity-60"
              disabled={
                props.regroupPending ||
                props.reconcilePending ||
                props.repairPending
              }
              type="button"
              onClick={props.onRegroupToday}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              {props.labels.regroupToday}
            </button>
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-semibold text-ink hover:border-accent hover:text-accent disabled:opacity-60"
              disabled={
                props.regroupPending ||
                props.reconcilePending ||
                props.repairPending
              }
              type="button"
              onClick={props.onRepairUploadSources}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              {props.labels.repairUploadSources}
            </button>
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
              disabled={
                props.reconcilePending ||
                props.regroupPending ||
                props.repairPending
              }
              type="button"
              onClick={props.onReconcile}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              {props.labels.scan}
            </button>
          </div>
        ) : null}
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <Metric
          label={props.labels.visibleSize}
          value={formatBytes(visibleSizeBytes)}
        />
        <Metric
          label={props.labels.shortSegments}
          value={String(shortSegmentCount)}
        />
        <Metric
          label={props.labels.protectedRecordings}
          value={String(protectedCount)}
        />
      </div>

      {props.reconcileResult ? (
        <p className="mt-3 text-sm text-muted">
          {props.labels.scanResult(
            props.reconcileResult.reconcile.imported,
            props.reconcileResult.reconcile.updated,
            props.reconcileResult.reconcile.skipped,
          )}
          {props.reconcileResult.reconcile.errors ? (
            <>
              {" "}
              Scan errors: {props.reconcileResult.reconcile.errors}
              {props.reconcileResult.reconcile.last_error
                ? ` (${props.reconcileResult.reconcile.last_error})`
                : ""}
            </>
          ) : null}{" "}
          {props.labels.uploadSourceDiscoverResult(
            props.reconcileResult.discover.created,
            props.reconcileResult.discover.ignored,
            props.reconcileResult.discover.merge_jobs_enqueued ?? 0,
            props.reconcileResult.discover.package_jobs_enqueued ?? 0,
          )}
        </p>
      ) : null}
      {props.reconcileError ? (
        <p className="mt-3 text-sm text-red-700">{props.labels.scanFailed}</p>
      ) : null}
      {props.regroupResult ? (
        <p className="mt-3 text-sm text-muted">
          {props.labels.regroupResult(props.regroupResult)}
        </p>
      ) : null}
      {props.regroupError ? (
        <p className="mt-3 text-sm text-red-700">
          {props.labels.regroupFailed}
        </p>
      ) : null}
      {props.repairResult ? (
        <p className="mt-3 text-sm text-muted">
          {props.labels.repairUploadSourcesResult(props.repairResult)}
        </p>
      ) : null}
      {props.repairError ? (
        <p className="mt-3 text-sm text-red-700">
          {props.labels.repairUploadSourcesFailed}
        </p>
      ) : null}

      <TableToolbar
        labels={props.labels}
        search={props.search}
        sort={props.sort}
        sortOptions={[
          { value: "started_desc", label: props.labels.sortNewest },
          { value: "started_asc", label: props.labels.sortOldest },
          { value: "duration_desc", label: props.labels.sortDuration },
          { value: "size_desc", label: props.labels.sortSize },
        ]}
        onSearchChange={props.onSearchChange}
        onSortChange={(value) => props.onSortChange(value as RecordingSortKey)}
      />

      <div className="mt-4 overflow-auto rounded-md border border-border">
        <table
          className="border-collapse text-left text-sm"
          style={{ minWidth: table.getTotalSize() }}
        >
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    className={`relative px-3 py-2 font-semibold ${header.column.id === "actions" ? "sticky right-0 z-20 bg-[#eef1eb] shadow-[-8px_0_12px_-12px_rgba(15,23,42,0.6)]" : ""}`}
                    style={{ width: header.getSize() }}
                  >
                    <div className="flex min-w-0 items-center justify-between gap-2">
                      <span className="min-w-0">
                        {header.isPlaceholder
                          ? null
                          : flexRender(
                              header.column.columnDef.header,
                              header.getContext(),
                            )}
                      </span>
                      {header.column.getCanResize() ? (
                        <button
                          aria-label={`Resize ${header.column.id}`}
                          className="absolute right-0 top-0 h-full w-2 cursor-col-resize touch-none border-r border-transparent hover:border-accent"
                          type="button"
                          onMouseDown={header.getResizeHandler()}
                          onTouchStart={header.getResizeHandler()}
                        />
                      ) : null}
                    </div>
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => {
              const recording = row.original;
              const isExpanded =
                expandedSourceId === recording.upload_source_id;
              const sourceId = recording.upload_source_id ?? 0;
              const originalSegmentsExpanded =
                sourceId > 0 && expandedSegmentSourceId === sourceId;
              return (
                <Fragment key={row.id}>
                  <tr key={row.id} className="bg-white">
                    {row.getVisibleCells().map((cell) => (
                      <td
                        key={cell.id}
                        className={`px-3 py-3 ${cell.column.id === "actions" ? "sticky right-0 z-10 bg-white shadow-[-8px_0_12px_-12px_rgba(15,23,42,0.6)]" : ""}`}
                        style={{ width: cell.column.getSize() }}
                      >
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext(),
                        )}
                      </td>
                    ))}
                  </tr>
                  {isExpanded ? (
                    <tr key={`${row.id}-segments`} className="bg-[#eef7f4]">
                      <td
                        className="p-0"
                        colSpan={table.getAllLeafColumns().length}
                      >
                        <div className="mx-3 mb-4 mt-0 rounded-md border border-accent/30 border-l-4 border-l-accent bg-[#f7fbf9] p-3 shadow-inner">
                          <div className="mb-3 flex min-w-0 flex-wrap items-center justify-between gap-3 border-b border-accent/20 pb-3">
                            <div className="min-w-0">
                              <p className="text-xs font-semibold uppercase text-accent">
                                {props.labels.recordingDetails}
                              </p>
                              <p className="mt-1 truncate text-sm font-semibold text-ink">
                                {recording.title ||
                                  recording.files?.[0]?.original_name ||
                                  props.labels.untitled}
                              </p>
                            </div>
                            <span className="text-xs text-muted">
                              {recording.profile_name} · {recording.room_id}
                            </span>
                          </div>
                          <div className="space-y-3">
                            {(recording.review_status ??
                              recording.upload_review_status) === "REQUIRED" &&
                            sourceId > 0 ? (
                              <UploadSourceReviewEditPanel
                                draft={props.editDrafts[sourceId] ?? ""}
                                editError={props.editError}
                                editPending={props.editPending}
                                labels={props.labels}
                                uploadSourceId={sourceId}
                                onApplyEdit={props.onApplyEdit}
                                onDraftChange={props.onEditDraftChange}
                              />
                            ) : null}
                            <UploadSourceOutputsTable
                              canDownload={props.canManageLocalFiles}
                              cosDownloadPending={props.cosDownloadPending}
                              cosDownloadPendingOutputId={
                                props.cosDownloadPendingOutputId
                              }
                              labels={props.labels}
                              outputs={recording.source_outputs ?? []}
                              reviewRequired={
                                (recording.review_status ??
                                  recording.upload_review_status) === "REQUIRED"
                              }
                              uploadSourceId={sourceId}
                              onDownloadLocalOutput={
                                props.onDownloadLocalOutput
                              }
                              onDownloadOutput={props.onDownloadOutput}
                            />
                            <DanmakuFilesTable
                              canDownload={props.canManageLocalFiles}
                              cosDownloadPending={props.cosFileDownloadPending}
                              cosDownloadPendingFileId={
                                props.cosFileDownloadPendingFileId
                              }
                              files={recording.danmaku_files ?? []}
                              labels={props.labels}
                              onDownloadFile={props.onDownloadFile}
                            />
                            <UploadSourceSegmentsTable
                              isExpanded={originalSegmentsExpanded}
                              labels={props.labels}
                              segments={recording.source_segments ?? []}
                              onToggle={() =>
                                setExpandedSegmentSourceId(
                                  originalSegmentsExpanded ? null : sourceId,
                                )
                              }
                            />
                          </div>
                        </div>
                      </td>
                    </tr>
                  ) : null}
                </Fragment>
              );
            })}
            {table.getRowModel().rows.length === 0 ? (
              <tr>
                <td
                  className="px-3 py-8 text-center text-muted"
                  colSpan={table.getAllLeafColumns().length}
                >
                  {props.isLoading
                    ? props.labels.loadingRecordings
                    : props.recordings.length === 0 && props.search
                      ? props.labels.emptyFiltered
                      : props.labels.noRecordings}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );
}

export function DanmakuFilesTable(props: {
  canDownload: boolean;
  cosDownloadPending: boolean;
  cosDownloadPendingFileId: number | null;
  files: RecordingFile[];
  labels: AdminCopy;
  onDownloadFile: (fileId: number) => void;
}) {
  return (
    <div className="rounded-md border border-border bg-white p-3">
      <h3 className="text-sm font-semibold">{props.labels.danmakuFiles}</h3>
      <div className="mt-3 overflow-auto">
        <table className="w-full min-w-[760px] border-collapse text-left text-xs">
          <thead className="bg-[#eef1eb] uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.file}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.completedAt}
              </th>
              <th className="px-3 py-2 font-semibold">{props.labels.size}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.cosStatus}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.actions}
              </th>
            </tr>
          </thead>
          <tbody>
            {props.files.map((file) => {
              const cosAvailable =
                props.canDownload && file.cos_status === "AVAILABLE";
              return (
                <tr key={file.id} className="align-top">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">
                      {file.original_name ||
                        file.relative_path.split("/").pop() ||
                        file.relative_path}
                    </p>
                    <p className="mt-1 break-all text-muted">
                      {file.relative_path}
                    </p>
                  </td>
                  <td className="px-3 py-3">
                    <TableDateTime value={file.closed_at ?? ""} />
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatBytes(file.size_bytes)}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatModuleUploadStatus(file.cos_status, props.labels)}
                  </td>
                  <td className="px-3 py-3">
                    {cosAvailable ? (
                      <button
                        className="inline-flex h-8 w-32 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                        disabled={
                          props.cosDownloadPending &&
                          props.cosDownloadPendingFileId === file.id
                        }
                        type="button"
                        onClick={() => props.onDownloadFile(file.id)}
                      >
                        <Download className="h-3.5 w-3.5" aria-hidden="true" />
                        {props.labels.downloadDanmakuFromCos}
                      </button>
                    ) : null}
                  </td>
                </tr>
              );
            })}
            {props.files.length === 0 ? (
              <tr>
                <td className="px-3 py-6 text-center text-muted" colSpan={5}>
                  {props.labels.noFile}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function UploadSourceSegmentsTable(props: {
  isExpanded: boolean;
  labels: AdminCopy;
  segments: UploadSourceSegment[];
  onToggle: () => void;
}) {
  return (
    <div className="rounded-md border border-border bg-white p-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">{props.labels.sourceSegments}</h3>
        <button
          className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
          type="button"
          onClick={props.onToggle}
        >
          {props.isExpanded ? (
            <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
          )}
          {props.isExpanded
            ? props.labels.hideSourceSegments
            : props.labels.showSourceSegments}
        </button>
      </div>
      {props.isExpanded ? (
        <div className="mt-3 overflow-auto">
          <table className="w-full min-w-[720px] border-collapse text-left text-xs">
            <thead className="bg-[#eef1eb] uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">{props.labels.file}</th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.startTime}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.completedAt}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.timeline}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.duration}
                </th>
                <th className="px-3 py-2 font-semibold">{props.labels.size}</th>
              </tr>
            </thead>
            <tbody>
              {props.segments.map((segment) => (
                <tr key={segment.id} className="align-top">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">
                      {segment.relative_path.split("/").pop() ??
                        segment.relative_path}
                    </p>
                    <p className="mt-1 break-all text-muted">
                      {segment.relative_path}
                    </p>
                  </td>
                  <td className="px-3 py-3">
                    <TableDateTime value={segment.source_started_at} />
                  </td>
                  <td className="px-3 py-3">
                    <TableDateTime value={segment.source_completed_at} />
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatTimeline(segment.timeline_start_ms)} -{" "}
                    {formatTimeline(segment.timeline_end_ms)}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatDuration(segment.duration_ms)}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatBytes(segment.size_bytes)}
                  </td>
                </tr>
              ))}
              {props.segments.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-muted" colSpan={6}>
                    {props.labels.noFile}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}

export function UploadSourceReviewEditPanel(props: {
  draft: string;
  editError: boolean;
  editPending: boolean;
  labels: AdminCopy;
  uploadSourceId: number;
  onApplyEdit: (uploadSourceId: number, draft: string) => void;
  onDraftChange: (uploadSourceId: number, value: string) => void;
}) {
  const cuts = parseEditCutDraft(props.draft);
  return (
    <div className="rounded-md border border-amber-200 bg-amber-50/50 p-3">
      <div className="flex flex-wrap items-end gap-3">
        <label className="min-w-[280px] flex-1 text-sm font-medium">
          {props.labels.editCuts}
          <textarea
            aria-label={props.labels.editCuts}
            className="mt-1 min-h-20 w-full rounded-md border border-border bg-white px-3 py-2 text-sm font-normal outline-none focus:border-accent"
            placeholder={props.labels.editCutsPlaceholder}
            value={props.draft}
            onChange={(event) =>
              props.onDraftChange(props.uploadSourceId, event.target.value)
            }
          />
        </label>
        <button
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-4 text-sm font-semibold text-white hover:bg-accent-strong disabled:opacity-60"
          disabled={props.editPending || cuts.length === 0}
          type="button"
          onClick={() => props.onApplyEdit(props.uploadSourceId, props.draft)}
        >
          <Save className="h-4 w-4" aria-hidden="true" />
          {props.labels.applyEdit}
        </button>
      </div>
      {props.editPending ? (
        <p className="mt-2 text-xs text-muted">{props.labels.editQueued}</p>
      ) : null}
      {props.editError ? (
        <p className="mt-2 text-xs text-red-700">{props.labels.editFailed}</p>
      ) : null}
    </div>
  );
}

export function UploadSourceOutputsTable(props: {
  canDownload: boolean;
  cosDownloadPending: boolean;
  cosDownloadPendingOutputId: number | null;
  labels: AdminCopy;
  outputs: UploadSourceOutput[];
  reviewRequired: boolean;
  uploadSourceId: number;
  onDownloadLocalOutput: (uploadSourceId: number, outputId: number) => void;
  onDownloadOutput: (uploadSourceId: number, outputId: number) => void;
}) {
  return (
    <div className="rounded-md border border-border bg-white p-3">
      <h3 className="text-sm font-semibold">{props.labels.sourceOutputs}</h3>
      <div className="mt-3 overflow-auto">
        <table className="w-full min-w-[1040px] border-collapse text-left text-xs">
          <thead className="bg-[#eef1eb] uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.file}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.timeline}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.duration}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.sourceSize}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.uploadedSize}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.compressionStatus}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.cosStatus}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.bilibiliStatus}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.actions}
              </th>
            </tr>
          </thead>
          <tbody>
            {props.outputs.map((output) => {
              const cosAvailable =
                props.canDownload && output.cos_status === "AVAILABLE";
              const localAvailable =
                props.canDownload &&
                props.reviewRequired &&
                output.status === "READY_TO_UPLOAD";
              const bilibiliURL =
                output.bilibili_status === "VERIFIED"
                  ? output.bilibili_url
                  : undefined;
              return (
                <tr key={output.id} className="align-top">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">
                      {output.relative_path.split("/").pop() ??
                        output.relative_path}
                    </p>
                    <p className="mt-1 break-all text-muted">
                      {output.relative_path}
                    </p>
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatTimeline(output.timeline_start_ms)} -{" "}
                    {formatTimeline(output.timeline_end_ms)}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatDuration(output.duration_ms)}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatBytes(
                      output.cos_source_size_bytes || output.size_bytes,
                    )}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {output.cos_uploaded_size_bytes
                      ? formatBytes(output.cos_uploaded_size_bytes)
                      : "-"}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatCompressionStatus(
                      output.cos_compression_status,
                      props.labels,
                    )}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>
                      {formatModuleUploadStatus(
                        output.cos_status,
                        props.labels,
                      )}
                    </p>
                    {output.cos_status === "FAILED" && output.cos_last_error ? (
                      <p className="mt-1 max-w-56 break-words text-red-700">
                        {output.cos_last_error}
                      </p>
                    ) : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>
                      {formatModuleUploadStatus(
                        output.bilibili_status,
                        props.labels,
                      )}
                    </p>
                    {output.bilibili_status === "FAILED" &&
                    output.bilibili_last_error ? (
                      <p className="mt-1 max-w-56 break-words text-red-700">
                        {output.bilibili_last_error}
                      </p>
                    ) : null}
                  </td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
                      {cosAvailable ? (
                        <button
                          className="inline-flex h-8 w-28 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                          disabled={
                            props.cosDownloadPending &&
                            props.cosDownloadPendingOutputId === output.id
                          }
                          type="button"
                          onClick={() =>
                            props.onDownloadOutput(
                              props.uploadSourceId,
                              output.id,
                            )
                          }
                        >
                          <Download
                            className="h-3.5 w-3.5"
                            aria-hidden="true"
                          />
                          {props.labels.downloadFromCos}
                        </button>
                      ) : null}
                      {localAvailable ? (
                        <button
                          className="inline-flex h-8 w-28 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                          type="button"
                          onClick={() =>
                            props.onDownloadLocalOutput(
                              props.uploadSourceId,
                              output.id,
                            )
                          }
                        >
                          <Download
                            className="h-3.5 w-3.5"
                            aria-hidden="true"
                          />
                          {props.labels.downloadLocal}
                        </button>
                      ) : null}
                      {bilibiliURL ? (
                        <a
                          className="inline-flex h-8 w-28 items-center justify-center whitespace-nowrap rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                          href={bilibiliURL}
                          rel="noreferrer"
                          target="_blank"
                        >
                          {props.labels.openBilibili}
                        </a>
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
            {props.outputs.length === 0 ? (
              <tr>
                <td className="px-3 py-6 text-center text-muted" colSpan={9}>
                  {props.labels.noFile}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
