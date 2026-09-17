import {
  type UploadSourceItem,
  type RecordingItem,
  type RecordingSortKey,
  type JobItem,
  type UploadSourceEditCut,
  type AdminCopy,
} from "../../shared/console/types";
import { includesSearch, chinaDateFromDate } from "../../shared/console/format";

export function uploadSourceToRecordingItem(
  source: UploadSourceItem,
): RecordingItem {
  const segments = source.segments ?? [];
  const outputs = source.outputs ?? [];
  const danmakuFiles = source.danmaku_files ?? [];
  return {
    id: segments[0]?.recording_id ?? source.id,
    upload_source_id: source.id,
    upload_source_status: source.status,
    local_cleanup_status: source.local_cleanup_status,
    local_deleted_at: source.local_deleted_at,
    bilibili_status: source.bilibili_status,
    bilibili_last_error: source.bilibili_last_error,
    cos_status: source.cos_status,
    cos_last_error: source.cos_last_error,
    output_recording_file_id: source.output_recording_file_id,
    output_relative_path: source.output_relative_path,
    last_error: source.last_error,
    review_status: source.review_status ?? "NONE",
    review_requested_at: source.review_requested_at,
    review_completed_at: source.review_completed_at,
    review_notes: source.review_notes,
    edit_decision_json: source.edit_decision_json,
    recording_profile_id: source.recording_profile_id,
    profile_name: source.profile_name,
    room_id: source.room_id,
    streamer_name: source.streamer_name,
    title: source.title,
    started_at: source.started_at,
    completed_at: source.completed_at,
    duration_ms: source.duration_ms,
    total_bytes:
      source.local_cleanup_status === "DELETED" ? 0 : source.total_bytes,
    recording_status: source.status,
    local_storage_status: source.output_relative_path
      ? "AVAILABLE"
      : source.status,
    local_protected: Boolean(source.local_protected),
    source_segments: segments,
    source_outputs: outputs,
    danmaku_files: danmakuFiles,
    files: segments.map((segment) => ({
      id: segment.recording_file_id,
      recording_id: segment.recording_id,
      relative_path: segment.relative_path,
      original_name:
        segment.relative_path.split("/").pop() ?? segment.relative_path,
      kind: "video",
      file_status:
        source.status === "READY_TO_UPLOAD" ? "CLOSED" : source.status,
      size_bytes: segment.size_bytes,
      duration_ms: segment.duration_ms,
      updated_at: "",
      closed_at: segment.source_completed_at,
    })),
  };
}

export function recordingToActiveRecordingItem(
  recording: RecordingItem,
): RecordingItem {
  return {
    ...recording,
    is_active_recording: true,
    upload_source_status:
      recording.recording_status === "ACTIVE"
        ? "ACTIVE_RECORDING"
        : recording.recording_status,
    review_status: recording.upload_review_status ?? "NONE",
    review_requested_at: recording.upload_review_requested_at,
    review_completed_at: recording.upload_review_completed_at,
    review_notes: recording.upload_review_notes,
    source_segments: [],
    source_outputs: [],
    danmaku_files: [],
    files: recording.files ?? [],
  };
}

export function filterRecordings(
  items: RecordingItem[],
  search: string,
  sort: RecordingSortKey,
): RecordingItem[] {
  const filtered = items.filter((recording) => {
    if (!search.trim()) {
      return true;
    }
    const firstFile = recording.files?.[0];
    return (
      includesSearch(recording.title, search) ||
      includesSearch(recording.profile_name, search) ||
      includesSearch(recording.room_id, search) ||
      includesSearch(recording.streamer_name, search) ||
      includesSearch(firstFile?.relative_path, search) ||
      includesSearch(firstFile?.original_name, search)
    );
  });
  return [...filtered].sort((left, right) => {
    if (sort === "started_asc") {
      return Date.parse(left.started_at) - Date.parse(right.started_at);
    }
    if (sort === "duration_desc") {
      return right.duration_ms - left.duration_ms;
    }
    if (sort === "size_desc") {
      return totalRecordingBytes(right) - totalRecordingBytes(left);
    }
    return Date.parse(right.started_at) - Date.parse(left.started_at);
  });
}

export function totalRecordingBytes(recording: RecordingItem): number {
  if (typeof recording.total_bytes === "number") {
    return recording.total_bytes;
  }
  return (recording.files ?? []).reduce(
    (total, file) => total + file.size_bytes,
    0,
  );
}

export function hasShortSegment(recording: RecordingItem): boolean {
  const segmentDurations =
    recording.source_segments
      ?.map((segment) => segment.duration_ms)
      .filter((value) => value > 0) ?? [];
  if (segmentDurations.some((value) => value < 3 * 60 * 1000)) {
    return true;
  }
  const durationMs =
    recording.duration_ms || recording.files?.[0]?.duration_ms || 0;
  return durationMs > 0 && durationMs < 3 * 60 * 1000;
}

export function currentUploadSourceMergeJob(
  recording: RecordingItem,
  jobs: JobItem[],
): JobItem | undefined {
  if (!recording.upload_source_id) {
    return undefined;
  }
  const businessKey = `upload-source:${recording.upload_source_id}:merge`;
  return jobs.find(
    (job) =>
      job.type === "MERGE_UPLOAD_SOURCE" && job.business_key === businessKey,
  );
}

export function currentUploadSourcePackageJob(
  recording: RecordingItem,
  jobs: JobItem[],
): JobItem | undefined {
  if (!recording.upload_source_id) {
    return undefined;
  }
  const businessKey = `upload-source:${recording.upload_source_id}:package`;
  return jobs.find(
    (job) =>
      job.type === "PACKAGE_UPLOAD_SOURCE" && job.business_key === businessKey,
  );
}

export function parseEditCutDraft(value: string): UploadSourceEditCut[] {
  const cuts: UploadSourceEditCut[] = [];
  for (const rawLine of value.split(/[\n,;]+/)) {
    const line = rawLine.trim();
    if (!line) {
      continue;
    }
    const match = line.match(/^(.+?)\s*[-~]\s*(.+)$/);
    if (!match) {
      return [];
    }
    const start = parseTimelineInput(match[1]);
    const end = parseTimelineInput(match[2]);
    if (start === null || end === null || end <= start) {
      return [];
    }
    cuts.push({ start_ms: start, end_ms: end });
  }
  return cuts.sort(
    (left, right) =>
      left.start_ms - right.start_ms || left.end_ms - right.end_ms,
  );
}

export function parseTimelineInput(value: string): number | null {
  const parts = value.trim().split(":");
  if (parts.length < 2 || parts.length > 3) {
    return null;
  }
  const numbers = parts.map((part) => Number(part));
  if (numbers.some((part) => !Number.isFinite(part) || part < 0)) {
    return null;
  }
  const [hours, minutes, seconds] =
    parts.length === 3 ? numbers : [0, numbers[0], numbers[1]];
  if (minutes >= 60 || seconds >= 60) {
    return null;
  }
  return Math.round(((hours * 60 + minutes) * 60 + seconds) * 1000);
}

export function formatUploadSourceStatus(
  value: string,
  labels: AdminCopy,
  mergeJob?: JobItem,
  packageJob?: JobItem,
): string {
  if (value === "ACTIVE_RECORDING" || value === "ACTIVE") {
    return labels.uploadSourceActiveRecording;
  }
  if (value === "READY_TO_UPLOAD") {
    return labels.uploadSourceReady;
  }
  if (value === "UPLOAD_COMPLETE") {
    return labels.uploadSourceComplete;
  }
  if (value === "UPLOAD_FAILED") {
    return labels.uploadSourceUploadFailed;
  }
  if (value === "UPLOADING") {
    return labels.uploadSourceUploading;
  }
  if (value === "WAITING_REVIEW") {
    return labels.uploadSourceWaitingReview;
  }
  if (value === "MERGE_PENDING") {
    if (mergeJob?.status === "RUNNING") {
      return labels.uploadSourceMerging;
    }
    if (mergeJob?.status === "SUCCEEDED") {
      return labels.uploadSourceMergeCompleteRefreshing;
    }
    return labels.uploadSourcePendingMerge;
  }
  if (value === "MERGE_FAILED") {
    return labels.uploadSourceMergeFailed;
  }
  if (value === "PACKAGE_PENDING") {
    if (packageJob?.status === "RUNNING") {
      return labels.uploadSourcePackaging;
    }
    if (packageJob?.status === "SUCCEEDED") {
      return labels.uploadSourcePackageCompleteRefreshing;
    }
    return labels.uploadSourcePendingPackage;
  }
  if (value === "PACKAGE_FAILED") {
    return labels.uploadSourcePackageFailed;
  }
  return value || labels.unknown;
}

export function deriveUploadSourceDisplayStatus(
  recording: RecordingItem,
): string {
  const sourceStatus =
    recording.upload_source_status ?? recording.recording_status;
  if (sourceStatus !== "READY_TO_UPLOAD") {
    return sourceStatus;
  }
  const destinationStatuses = [
    recording.bilibili_status,
    recording.cos_status,
  ].filter(
    (status): status is string => Boolean(status) && status !== "DISABLED",
  );
  if (destinationStatuses.some((status) => status === "FAILED")) {
    return "UPLOAD_FAILED";
  }
  if (
    destinationStatuses.some(
      (status) => status === "UPLOADING" || status === "VERIFYING",
    )
  ) {
    return "UPLOADING";
  }
  if (
    destinationStatuses.length > 0 &&
    destinationStatuses.every(
      (status) => status === "VERIFIED" || status === "AVAILABLE",
    )
  ) {
    return "UPLOAD_COMPLETE";
  }
  return sourceStatus;
}

export function currentChinaDate(): string {
  return chinaDateFromDate(new Date());
}

export function latestRecordingChinaDate(
  recordings: RecordingItem[],
): string | null {
  let latest: Date | null = null;
  for (const recording of recordings) {
    const started = new Date(recording.started_at);
    if (Number.isNaN(started.getTime())) {
      continue;
    }
    if (!latest || started > latest) {
      latest = started;
    }
  }
  return latest ? chinaDateFromDate(latest) : null;
}
