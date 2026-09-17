import type * as Contracts from "../api/contracts.generated";
import { uiCopy } from "./copy";

export type User = Contracts.User;

export type ManagerPolicy = Contracts.ManagerPolicy;

export type PolicyFlag = Exclude<keyof ManagerPolicy, "updated_at">;

export type Account = Contracts.Account;

export type MeResponse = {
  policy?: ManagerPolicy;
  user: User;
};

export type AccountListResponse = {
  items: Account[] | null;
  total?: number;
};

export type HealthResponse = {
  status: string;
  release_sha: string;
};

export type RecordingSettings = Contracts.RecordingSettings;

export type RecordingProfile = Contracts.RecordingProfile;

export type ProfileListResponse = {
  items: RecordingProfile[] | null;
  total?: number;
};

export type RecordingFile = Contracts.RecordingFile;

export type RecordingItem = {
  id: number;
  is_active_recording?: boolean;
  upload_source_id?: number;
  upload_source_status?: string;
  local_cleanup_status?: string;
  local_deleted_at?: string;
  bilibili_status?: string;
  bilibili_last_error?: string;
  cos_status?: string;
  cos_last_error?: string;
  output_recording_file_id?: number;
  output_relative_path?: string;
  last_error?: string;
  recording_profile_id: number;
  profile_name: string;
  room_id: string;
  streamer_name: string;
  title?: string;
  started_at: string;
  completed_at?: string;
  duration_ms: number;
  total_bytes?: number;
  recording_status: string;
  local_storage_status: string;
  local_protected: boolean;
  upload_review_status?: string;
  upload_review_requested_at?: string;
  upload_review_completed_at?: string;
  upload_review_notes?: string;
  review_status?: string;
  review_requested_at?: string;
  review_completed_at?: string;
  review_notes?: string;
  edit_decision_json?: string;
  source_segments?: UploadSourceSegment[];
  source_outputs?: UploadSourceOutput[];
  danmaku_files?: RecordingFile[];
  files: RecordingFile[] | null;
};

export type UploadSourceSegment = Contracts.UploadSourceSegment;

export type UploadSourceOutput = Contracts.UploadSourceOutput;

export type UploadSourceEditCut = {
  start_ms: number;
  end_ms: number;
};

export type UploadSourceItem = Contracts.UploadSourceItem;

export type UploadSourceListResponse = {
  items: UploadSourceItem[] | null;
  total: number;
  merge_gap_threshold_seconds: number;
};

export type RecordingListResponse = {
  items: RecordingItem[] | null;
  total: number;
};

export type UploadSourceDiscoverResult = {
  created: number;
  ignored: number;
  delayed?: number;
  merge_jobs_enqueued?: number;
  package_jobs_enqueued?: number;
  merge_gap_threshold_seconds: number;
};

export type UploadSourceRegroupResult = {
  replaced_sources: number;
  created_sources: number;
  cancelled_jobs: number;
  blocked?: Array<{
    upload_source_ids: number[];
    reason: string;
  }>;
  merge_gap_threshold_seconds: number;
};

export type UploadSourceRepairResult = {
  checked: number;
  reset_to_merge: number;
  reset_to_package: number;
  outputs_marked_missing: number;
  upload_jobs_cancelled: number;
  merge_jobs_reset: number;
  package_jobs_reset: number;
  source_missing_blocks: number;
  bilibili_publications_reset: number;
};

export type RecordingRegroupResult = {
  china_date: string;
  items: UploadSourceRegroupResult[];
};

export type ReconcileResult = {
  scanned_files: number;
  imported: number;
  updated: number;
  skipped: number;
  errors?: number;
  last_error?: string;
};

export type RecordingScanResult = {
  reconcile: ReconcileResult;
  discover: UploadSourceDiscoverResult;
};

export type COSDownloadURLResponse = {
  url: string;
  expires_at: string;
  object_key: string;
};

export type LocalStorageStatus = Contracts.LocalStorageStatus;

export type LocalStorageSettings = Contracts.LocalStorageSettings;

export type CleanupCandidate = Contracts.CleanupCandidate;

export type CleanupCandidateListResponse = {
  items: CleanupCandidate[] | null;
  total?: number;
  preview_reclaimable_bytes: number;
};

export type CleanupRunResult = Contracts.CleanupRunResult;

export type JobItem = Contracts.Job;

export type JobListResponse = {
  items: JobItem[] | null;
  total?: number;
};

export type Credential = Contracts.Credential;

export type CredentialListResponse = {
  items: Credential[] | null;
  total?: number;
};

export type BilibiliPublishingConfig = Contracts.BilibiliPublishingConfig;

export type COSStorageConfig = Contracts.COSStorageConfig;

export type SiteTLSSettings = Contracts.SiteTLSSettings;

export type SiteTLSForm = {
  enabled: boolean;
  credential_id: string;
  primary_domain: string;
  additional_domains: string;
};

export type SongSettings = Contracts.SongSettings;

export type SongSettingsForm = {
  enabled: boolean;
  credential_id: string;
  region: string;
  container_id: string;
  destination_cos_storage_profile_id: string;
  songs_prefix: string;
  boundary_padding_ms: number;
  algorithm_version: string;
};

export type SongAnalysisSource = Contracts.SongAnalysisSource;

export type SongAnalysisRun = Contracts.SongAnalysisRun;

export type SongSourceListResponse = {
  items: SongAnalysisSource[] | null;
  total?: number;
};

export type SongRunListResponse = {
  items: SongAnalysisRun[] | null;
  total?: number;
};

export type RecognizedSong = Contracts.RecognizedSong;

export type RecognizedSongListResponse = {
  items: RecognizedSong[] | null;
  total?: number;
};

export type UploadModuleReconcileResult = {
  publications_created: number;
  bilibili_jobs_created: number;
  cos_objects_created: number;
  cos_jobs_created: number;
  cos_file_objects_created?: number;
  cos_file_jobs_created?: number;
};

export type ProfileForm = {
  owner_user_id: string;
  name: string;
  room_id: string;
  streamer_name: string;
  streamer_uid: string;
  timezone: string;
  enabled: boolean;
  public_enabled: boolean;
  public_slug: string;
  auto_record: boolean;
  quality: string;
  record_danmaku: boolean;
  segment_duration_sec: number;
  finalize_grace_period_sec: number;
};

export type AdminPage =
  | "overview"
  | "profiles"
  | "recordings"
  | "uploads"
  | "songs"
  | "jobs"
  | "system"
  | "accounts"
  | "me";

export type Language = "zh" | "en";

export type RecordingSortKey =
  | "started_desc"
  | "started_asc"
  | "duration_desc"
  | "size_desc";

export type ProfileSortKey = "name_asc" | "room_asc";

export type AccountSortKey = "username_asc" | "role_asc";

export type JobSortKey = "updated_desc" | "run_after_asc" | "status_asc";

export type AccountForm = {
  username: string;
  password: string;
  enabled: boolean;
  policy: ManagerPolicy;
};

export type AccountEditForm = {
  username: string;
  password: string;
  enabled: boolean;
};

export type CredentialForm = {
  platform: "bilibili" | "tencent_cos";
  account_label: string;
  external_uid: string;
  secret: string;
};

export type UploadSettingsForm = {
  profile_id: string;
  bilibili_enabled: boolean;
  bilibili_credential_id: string;
  bilibili_title_template: string;
  bilibili_description_template: string;
  bilibili_tags: string;
  bilibili_copyright: number;
  bilibili_source: string;
  bilibili_upload_limit: number;
  cos_enabled: boolean;
  cos_credential_id: string;
  cos_region: string;
  cos_bucket: string;
  cos_prefix: string;
  cos_max_managed_gb: number;
};

export type AdminCopy = (typeof uiCopy)[Language];
