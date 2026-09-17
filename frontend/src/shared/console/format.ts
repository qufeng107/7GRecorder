import {
  type RecordingProfile,
  type ProfileForm,
  type Account,
  type AccountEditForm,
  type User,
  type ManagerPolicy,
  type PolicyFlag,
  type AdminCopy,
} from "./types";

export const UPLOAD_SOURCE_MERGE_GAP_SECONDS = 600;

export function profileToForm(profile: RecordingProfile): ProfileForm {
  return {
    owner_user_id: String(profile.owner_user_id),
    name: profile.name,
    room_id: profile.room_id,
    streamer_name: profile.streamer_name,
    streamer_uid: profile.streamer_uid ?? "",
    timezone: profile.timezone,
    enabled: profile.enabled,
    public_enabled: profile.public_enabled,
    public_slug: profile.public_slug ?? "",
    auto_record: profile.recording_settings.auto_record,
    quality: profile.recording_settings.quality,
    record_danmaku: profile.recording_settings.record_danmaku,
    segment_duration_sec: profile.recording_settings.segment_duration_sec,
    finalize_grace_period_sec:
      profile.recording_settings.finalize_grace_period_sec,
  };
}

export function accountToEditForm(account: Account): AccountEditForm {
  return {
    username: account.username,
    password: "",
    enabled: account.enabled,
  };
}

export function hasManagerPermission(
  user: User | undefined,
  policy: ManagerPolicy | undefined,
  key: PolicyFlag,
): boolean {
  if (!user) {
    return false;
  }
  if (user.role === "SUPER_ADMIN") {
    return true;
  }
  return Boolean(policy?.[key]);
}

export function includesSearch(
  value: string | number | undefined,
  search: string,
): boolean {
  return String(value ?? "")
    .toLowerCase()
    .includes(search.trim().toLowerCase());
}

export function formatBytes(value: number): string {
  if (!value) {
    return "-";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(size >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

export function bytesToGB(value: number): number {
  return Math.max(0, Math.round(value / 1024 / 1024 / 1024));
}

export function gbToBytes(value: number): number {
  return Math.max(0, Math.round(value * 1024 * 1024 * 1024));
}

export function formatJobType(value: string, labels: AdminCopy): string {
  if (value === "SYNC_RECORDER_PROFILE") {
    return labels.jobSyncRecorderProfile;
  }
  if (value === "MERGE_UPLOAD_SOURCE") {
    return labels.jobMergeUploadSource;
  }
  if (value === "PACKAGE_UPLOAD_SOURCE") {
    return labels.jobPackageUploadSource;
  }
  if (value === "APPLY_UPLOAD_SOURCE_EDIT") {
    return labels.jobApplyUploadSourceEdit;
  }
  if (value === "UPLOAD_BILIBILI") {
    return labels.jobUploadBilibili;
  }
  if (value === "UPLOAD_COS_OBJECT") {
    return labels.jobUploadCOS;
  }
  if (value === "UPLOAD_COS_RECORDING_FILE") {
    return labels.jobUploadCOSRecordingFile;
  }
  if (value === "SYNC_SITE_TLS") {
    return labels.jobSyncSiteTLS;
  }
  return value;
}

export function formatJobStatus(value: string, labels: AdminCopy): string {
  if (value === "PENDING") {
    return labels.jobStatusPending;
  }
  if (value === "RUNNING") {
    return labels.jobStatusRunning;
  }
  if (value === "SUCCEEDED") {
    return labels.jobStatusSucceeded;
  }
  if (value === "FAILED") {
    return labels.jobStatusFailed;
  }
  if (value === "CANCELLED") {
    return labels.jobStatusCancelled;
  }
  return value;
}

export function formatModuleUploadStatus(
  value: string | undefined,
  labels: AdminCopy,
): string {
  if (value === "WAITING_REVIEW") {
    return labels.uploadSourceWaitingReview;
  }
  if (value === "DISABLED") {
    return labels.uploadStatusDisabled;
  }
  if (value === "WAITING_SOURCE") {
    return labels.uploadStatusWaitingSource;
  }
  if (value === "PENDING") {
    return labels.uploadStatusPending;
  }
  if (value === "UPLOADING" || value === "VERIFYING") {
    return labels.uploadStatusUploading;
  }
  if (value === "AVAILABLE") {
    return labels.uploadStatusAvailable;
  }
  if (value === "VERIFIED") {
    return labels.uploadStatusVerified;
  }
  if (value === "FAILED") {
    return labels.uploadStatusFailed;
  }
  return value || labels.unknown;
}

export function formatCompressionStatus(
  value: string | undefined,
  labels: AdminCopy,
): string {
  if (value === "DISABLED") {
    return labels.compressionStatusDisabled;
  }
  if (value === "PENDING") {
    return labels.compressionStatusPending;
  }
  if (value === "COMPRESSING") {
    return labels.compressionStatusCompressing;
  }
  if (value === "COMPRESSED") {
    return labels.compressionStatusCompressed;
  }
  if (value === "SKIPPED_LOW_GAIN") {
    return labels.compressionStatusSkippedLowGain;
  }
  if (value === "FAILED") {
    return labels.compressionStatusFailed;
  }
  return value || "-";
}

export function formatTimeline(value: number): string {
  const totalSeconds = Math.max(0, Math.round(value / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  return [hours, minutes, seconds]
    .map((part) => String(part).padStart(2, "0"))
    .join(":");
}

export function formatDuration(value: number): string {
  if (!value) {
    return "-";
  }
  const totalSeconds = Math.round(value / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

export function formatDateTime(value: string, labels: AdminCopy): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const formatted = new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
  return `${formatted} ${labels.chinaTime}`;
}

export function formatChinaDateParts(value: string): {
  date: string;
  time: string;
} {
  if (!value) {
    return { date: "-", time: "-" };
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return { date: value, time: "-" };
  }
  const parts = new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).formatToParts(date);
  const valueFor = (type: string) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return {
    date: `${valueFor("year")}/${valueFor("month")}/${valueFor("day")}`,
    time: `${valueFor("hour")}:${valueFor("minute")}:${valueFor("second")}`,
  };
}

export function chinaDateFromTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return chinaDateFromDate(date);
}

export function chinaDateFromDate(date: Date): string {
  const parts = new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const valueFor = (type: string) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return `${valueFor("year")}-${valueFor("month")}-${valueFor("day")}`;
}
