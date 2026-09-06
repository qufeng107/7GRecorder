import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity,
  Archive,
  ArchiveRestore,
  Database,
  Download,
  FileVideo,
  HardDrive,
  LayoutDashboard,
  Lock,
  LogIn,
  LogOut,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings,
  ShieldCheck,
  Trash2,
  Unlock,
  UserCircle,
  UserPlus,
  Users,
  X
} from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

type User = {
  id: number;
  username: string;
  role: "SUPER_ADMIN" | "MANAGER";
  enabled: boolean;
};

type ManagerPolicy = {
  can_edit_recording_profile: boolean;
  can_edit_bilibili_module: boolean;
  can_edit_cos_module: boolean;
  can_edit_netease_module: boolean;
  can_manage_local_files: boolean;
  updated_at?: string;
};

type PolicyFlag = Exclude<keyof ManagerPolicy, "updated_at">;

type Account = User & {
  profile_count: number;
  policy?: ManagerPolicy;
};

type MeResponse = {
  policy?: ManagerPolicy;
  user: User;
};

type AccountListResponse = {
  items: Account[] | null;
  total?: number;
};

type HealthResponse = {
  status: string;
  release_sha: string;
};

type RecordingSettings = {
  auto_record: boolean;
  quality: string;
  record_danmaku: boolean;
  segment_duration_sec: number;
  finalize_grace_period_sec: number;
};

type RecordingProfile = {
  id: number;
  name: string;
  owner_user_id: number;
  owner_username?: string;
  platform: string;
  room_id: string;
  streamer_name: string;
  streamer_uid?: string;
  timezone: string;
  enabled: boolean;
  public_enabled: boolean;
  public_slug?: string;
  archived_at?: string;
  recording_settings: RecordingSettings;
  runtime: {
    stream_status: string;
    recorder_status: string;
    sync_status: string;
  };
};

type ProfileListResponse = {
  items: RecordingProfile[] | null;
  total?: number;
};

type RecordingFile = {
  id: number;
  recording_id: number;
  relative_path: string;
  original_name: string;
  kind: string;
  file_status: string;
  size_bytes: number;
  duration_ms: number;
  closed_at?: string;
};

type RecordingItem = {
  id: number;
  recording_profile_id: number;
  profile_name: string;
  room_id: string;
  streamer_name: string;
  title?: string;
  started_at: string;
  completed_at?: string;
  duration_ms: number;
  recording_status: string;
  local_storage_status: string;
  local_protected: boolean;
  files: RecordingFile[] | null;
};

type RecordingListResponse = {
  items: RecordingItem[] | null;
  total?: number;
};

type ReconcileResult = {
  scanned_files: number;
  imported: number;
  updated: number;
  skipped: number;
};

type LocalStorageStatus = {
  data_root: string;
  disk_total_bytes: number;
  disk_free_bytes: number;
  disk_available_bytes: number;
  indexed_video_bytes: number;
  indexed_video_files: number;
  protected_recordings: number;
  completed_recordings: number;
  settings_configured: boolean;
  health: "HEALTHY" | "WARNING" | "CRITICAL";
  need_reclaim_bytes: number;
  target_video_bytes: number;
  settings: LocalStorageSettings;
};

type LocalStorageSettings = {
  max_recording_bytes: number;
  min_system_free_bytes: number;
  cleanup_target_ratio: number;
  absolute_emergency_free_bytes: number;
  updated_at?: string;
};

type CleanupCandidate = {
  recording_id: number;
  profile_name: string;
  room_id: string;
  streamer_name: string;
  title?: string;
  started_at: string;
  completed_at?: string;
  duration_ms: number;
  file_count: number;
  reclaimable_bytes: number;
};

type CleanupCandidateListResponse = {
  items: CleanupCandidate[] | null;
  total?: number;
  preview_reclaimable_bytes: number;
};

type CleanupRunResult = {
  deleted_recordings: number;
  deleted_files: number;
  reclaimed_bytes: number;
  skipped_recordings: number;
};

type JobItem = {
  id: number;
  recording_profile_id?: number;
  recording_id?: number;
  recording_file_id?: number;
  type: string;
  resource_class: string;
  business_key?: string;
  status: string;
  priority: number;
  attempts: number;
  max_attempts: number;
  run_after: string;
  locked_at?: string;
  heartbeat_at?: string;
  locked_by?: string;
  last_error_class?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
  profile_name?: string;
  owner_username?: string;
};

type JobListResponse = {
  items: JobItem[] | null;
  total?: number;
};

type ProfileForm = {
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

type AdminPage = "overview" | "profiles" | "recordings" | "jobs" | "system" | "accounts" | "me";
type Language = "zh" | "en";
type RecordingSortKey = "started_desc" | "started_asc" | "duration_desc" | "size_desc";
type ProfileSortKey = "name_asc" | "room_asc";
type AccountSortKey = "username_asc" | "role_asc";
type JobSortKey = "updated_desc" | "run_after_asc" | "status_asc";

type AccountForm = {
  username: string;
  password: string;
  enabled: boolean;
  policy: ManagerPolicy;
};

type AccountEditForm = {
  username: string;
  password: string;
  enabled: boolean;
};

const emptyProfileForm: ProfileForm = {
  owner_user_id: "",
  name: "",
  room_id: "",
  streamer_name: "",
  streamer_uid: "",
  timezone: "Asia/Shanghai",
  enabled: true,
  public_enabled: false,
  public_slug: "",
  auto_record: true,
  quality: "original",
  record_danmaku: true,
  segment_duration_sec: 1800,
  finalize_grace_period_sec: 300
};

const defaultManagerPolicy: ManagerPolicy = {
  can_edit_recording_profile: true,
  can_edit_bilibili_module: true,
  can_edit_cos_module: true,
  can_edit_netease_module: true,
  can_manage_local_files: true
};

const emptyAccountForm: AccountForm = {
  username: "",
  password: "",
  enabled: true,
  policy: defaultManagerPolicy
};

const emptyAccountEditForm: AccountEditForm = {
  username: "",
  password: "",
  enabled: true
};

const uiCopy = {
  zh: {
    appName: "7GRecorder 管理后台",
    title: "录播控制台",
    navLabel: "管理分区",
    nav: {
      overview: "总览",
      profiles: "录制配置",
      recordings: "录像文件",
      jobs: "任务",
      accounts: "账号管理",
      system: "系统设置"
    },
    statusRows: [
      { label: "录制核心", value: "配置已就绪", icon: Activity },
      { label: "SQLite", value: "部署时自动迁移", icon: Database },
      { label: "本地存储", value: "始终启用", icon: HardDrive },
      { label: "可选模块", value: "配置后启用", icon: Archive },
      { label: "部署", value: "仅 main 分支发布生产", icon: ShieldCheck }
    ],
    language: "语言",
    chinese: "中文",
    english: "English",
    myAccount: "我的账号",
    signOut: "退出登录",
    signIn: "登录",
    session: "会话",
    username: "用户名",
    password: "密码",
    role: "角色",
    status: "状态",
    enabled: "启用",
    disabled: "停用",
    allowed: "允许",
    blocked: "禁止",
    access: "权限",
    systemSettings: "系统设置",
    profiles: "录制配置",
    allOwners: "全部账号",
    policyUnavailable: "账号权限暂不可用。",
    loginFailed: "登录失败，请检查用户名和密码。",
    newProfile: "新建配置",
    editProfile: "编辑配置",
    archivedProfile: "已归档配置",
    owner: "所属账号",
    currentOwner: "当前所属账号",
    name: "名称",
    roomId: "直播间 ID",
    streamer: "主播",
    streamerUid: "主播 UID",
    timezone: "时区",
    publicSlug: "公开地址",
    quality: "画质",
    segmentSeconds: "分段秒数",
    finalizeGraceSeconds: "收尾宽限秒数",
    autoRecord: "自动录制",
    recordDanmaku: "录制弹幕",
    publicPage: "公开页面",
    save: "保存",
    create: "创建",
    cancel: "取消",
    close: "关闭",
    restoreProfile: "恢复配置",
    archiveProfile: "归档配置",
    confirmArchive: "确认归档",
    profileSaveFailed: "配置保存失败，请检查直播间是否重复以及必填项。",
    recordingProfiles: "录制配置",
    total: (count: number) => `共 ${count} 条`,
    new: "新建",
    ownerColumn: "所属账号",
    room: "直播间",
    runtime: "运行",
    sync: "同步",
    actions: "操作",
    edit: "编辑",
    archived: "已归档",
    noProfiles: "暂无录制配置。",
    newManager: "新建管理员",
    initialPassword: "初始密码",
    accountCreationFailed: "账号创建失败，请检查用户名和密码。",
    accounts: "账号",
    account: "账号",
    noAccounts: "暂无账号。",
    editAccount: "编辑账号",
    accountSaveFailed: "账号保存失败，请检查用户名、密码或账号状态。",
    newPassword: "新密码",
    newPasswordHint: "留空表示不修改密码。",
    saveAccount: "保存账号",
    currentAccount: "当前账号",
    noAction: "无可用操作",
    disable: "停用",
    enable: "启用",
    editProfiles: "编辑录制配置",
    bilibiliConfig: "Bilibili 配置",
    cosConfig: "COS 配置",
    neteaseConfig: "网易云配置",
    localFiles: "本地文件",
    localStorage: "本地存储",
    checkingStorage: "正在检查存储。",
    indexedVideos: "已索引视频",
    indexedSize: "已索引大小",
    diskAvailable: "磁盘可用",
    protected: "已保护",
    health: "健康状态",
    needReclaim: "需回收",
    previewReclaimable: "预估可回收",
    diskSummary: (used: number, total: string, completed: number, configured: boolean) =>
      `磁盘已用 ${used}%，总计 ${total}。已完成录像 ${completed} 条。设置：${configured ? "已配置" : "使用默认值"}。`,
    storageSettings: "存储设置",
    maxRecordingGB: "录像上限 GB",
    minFreeGB: "最低空闲 GB",
    emergencyFreeGB: "紧急保留 GB",
    cleanupTargetPercent: "清理目标 %",
    storageSaveFailed: "存储设置保存失败，请检查阈值。",
    cleanupPreview: "清理预览",
    oldestUnprotected: "最早的未保护已完成录像",
    recording: "录像",
    profile: "配置",
    closed: "结束时间",
    files: "文件数",
    reclaimable: "可回收",
    untitled: "未命名",
    noCleanupCandidates: "暂无可清理候选。",
    runCleanup: "执行清理",
    cleanupConfirm: "将删除最旧的未保护已完成录像文件，并保留数据库记录。确认执行？",
    cleanupResult: (recordings: number, files: number, bytes: string, skipped: number) =>
      `清理完成：删除 ${recordings} 条录像、${files} 个文件，回收 ${bytes}，跳过 ${skipped} 条。`,
    cleanupFailed: "清理失败，请查看服务器日志。",
    jobs: "任务",
    job: "任务",
    jobType: "类型",
    resourceClass: "资源",
    runAfter: "计划时间",
    attempts: "尝试",
    lastError: "最近错误",
    retry: "重试",
    retryJob: "重试任务",
    cancelJob: "取消任务",
    jobsFailed: "任务加载失败，请查看服务器日志。",
    noJobs: "暂无任务。",
    recordings: "录像文件",
    scan: "扫描",
    scanResult: (imported: number, updated: number, skipped: number) =>
      `扫描：新增 ${imported}，更新 ${updated}，忽略 ${skipped}。`,
    scanFailed: "扫描失败，请查看服务器日志。",
    startTime: "录制时间",
    completedAt: "完成时间",
    duration: "时长",
    size: "大小",
    path: "路径",
    fileStatus: "文件状态",
    noFile: "无文件",
    unprotect: "取消保护",
    protect: "保护",
    download: "下载",
    details: "详情",
    recordingDetails: "录像详情",
    shortRecording: "短片段",
    recordingStatus: "录像状态",
    localStorageStatus: "本地状态",
    file: "文件",
    fileKind: "文件类型",
    fileSize: "文件大小",
    filePath: "文件路径",
    visibleSize: "当前列表大小",
    shortSegments: "短片段",
    protectedRecordings: "受保护录像",
    loadingRecordings: "正在加载录像。",
    noRecordings: "暂无已索引录像。",
    api: "API",
    release: "版本",
    checking: "检查中",
    unknown: "未知",
    chinaTime: "中国时间",
    search: "搜索",
    searchPlaceholder: "搜索名称、直播间、路径",
    sortBy: "排序",
    sortNewest: "录制时间：新到旧",
    sortOldest: "录制时间：旧到新",
    sortDuration: "时长：长到短",
    sortSize: "大小：大到小",
    sortName: "名称：A 到 Z",
    sortRoom: "直播间：小到大",
    sortUsername: "用户名：A 到 Z",
    sortRole: "角色：A 到 Z",
    sortUpdated: "更新时间：新到旧",
    sortRunAfter: "计划时间：近到远",
    sortStatus: "状态：A 到 Z",
    emptyFiltered: "没有匹配结果。"
  },
  en: {
    appName: "7GRecorder Admin",
    title: "Recorder Console",
    navLabel: "Admin sections",
    nav: {
      overview: "Overview",
      profiles: "Profiles",
      recordings: "Recordings",
      jobs: "Jobs",
      accounts: "Accounts",
      system: "System Settings"
    },
    statusRows: [
      { label: "Recording Core", value: "Profiles ready", icon: Activity },
      { label: "SQLite", value: "Migrated on deploy", icon: Database },
      { label: "Local Storage", value: "Always enabled", icon: HardDrive },
      { label: "Optional Modules", value: "Disabled until configured", icon: Archive },
      { label: "Deployment", value: "main-only production", icon: ShieldCheck }
    ],
    language: "Language",
    chinese: "中文",
    english: "English",
    myAccount: "My Account",
    signOut: "Sign out",
    signIn: "Sign in",
    session: "Session",
    username: "Username",
    password: "Password",
    role: "Role",
    status: "Status",
    enabled: "ENABLED",
    disabled: "DISABLED",
    allowed: "Allowed",
    blocked: "Blocked",
    access: "Access",
    systemSettings: "System Settings",
    profiles: "Profiles",
    allOwners: "All owners",
    policyUnavailable: "Access policy is not available.",
    loginFailed: "Login failed. Check the credentials.",
    newProfile: "New Profile",
    editProfile: "Edit Profile",
    archivedProfile: "Archived profile",
    owner: "Owner",
    currentOwner: "Current owner",
    name: "Name",
    roomId: "Room ID",
    streamer: "Streamer",
    streamerUid: "Streamer UID",
    timezone: "Timezone",
    publicSlug: "Public Slug",
    quality: "Quality",
    segmentSeconds: "Segment Seconds",
    finalizeGraceSeconds: "Finalize Grace Seconds",
    autoRecord: "Auto Record",
    recordDanmaku: "Record Danmaku",
    publicPage: "Public Page",
    save: "Save",
    create: "Create",
    cancel: "Cancel",
    close: "Close",
    restoreProfile: "Restore profile",
    archiveProfile: "Archive profile",
    confirmArchive: "Confirm archive",
    profileSaveFailed: "Profile save failed. Check unique room and required fields.",
    recordingProfiles: "Recording Profiles",
    total: (count: number) => `${count} total`,
    new: "New",
    ownerColumn: "Owner",
    room: "Room",
    runtime: "Runtime",
    sync: "Sync",
    actions: "Actions",
    edit: "Edit",
    archived: "Archived",
    noProfiles: "No profiles yet.",
    newManager: "New Manager",
    initialPassword: "Initial Password",
    accountCreationFailed: "Account creation failed. Check username and password.",
    accounts: "Accounts",
    account: "Account",
    noAccounts: "No accounts yet.",
    editAccount: "Edit Account",
    accountSaveFailed: "Account save failed. Check username, password, or account status.",
    newPassword: "New Password",
    newPasswordHint: "Leave blank to keep the current password.",
    saveAccount: "Save Account",
    currentAccount: "Current account",
    noAction: "No actions",
    disable: "Disable",
    enable: "Enable",
    editProfiles: "Edit profiles",
    bilibiliConfig: "Bilibili config",
    cosConfig: "COS config",
    neteaseConfig: "NetEase config",
    localFiles: "Local files",
    localStorage: "Local Storage",
    checkingStorage: "Checking storage.",
    indexedVideos: "Indexed Videos",
    indexedSize: "Indexed Size",
    diskAvailable: "Disk Available",
    protected: "Protected",
    health: "Health",
    needReclaim: "Need Reclaim",
    previewReclaimable: "Preview Reclaimable",
    diskSummary: (used: number, total: string, completed: number, configured: boolean) =>
      `Disk used: ${used}% of ${total}. Completed recordings: ${completed}. Settings: ${
        configured ? "configured" : "derived default"
      }.`,
    storageSettings: "Storage Settings",
    maxRecordingGB: "Max Recording GB",
    minFreeGB: "Min Free GB",
    emergencyFreeGB: "Emergency Free GB",
    cleanupTargetPercent: "Cleanup Target %",
    storageSaveFailed: "Storage settings save failed. Check the thresholds.",
    cleanupPreview: "Cleanup Preview",
    oldestUnprotected: "Oldest unprotected completed recordings",
    recording: "Recording",
    profile: "Profile",
    closed: "Closed",
    files: "Files",
    reclaimable: "Reclaimable",
    untitled: "Untitled",
    noCleanupCandidates: "No cleanup candidates.",
    runCleanup: "Run Cleanup",
    cleanupConfirm: "This will delete the oldest unprotected completed local recording files while keeping database records. Continue?",
    cleanupResult: (recordings: number, files: number, bytes: string, skipped: number) =>
      `Cleanup finished: deleted ${recordings} recordings, ${files} files, reclaimed ${bytes}, skipped ${skipped}.`,
    cleanupFailed: "Cleanup failed. Check server logs.",
    jobs: "Jobs",
    job: "Job",
    jobType: "Type",
    resourceClass: "Resource",
    runAfter: "Run After",
    attempts: "Attempts",
    lastError: "Last Error",
    retry: "Retry",
    retryJob: "Retry job",
    cancelJob: "Cancel job",
    jobsFailed: "Jobs failed to load. Check server logs.",
    noJobs: "No jobs yet.",
    recordings: "Recordings",
    scan: "Scan",
    scanResult: (imported: number, updated: number, skipped: number) =>
      `Scan: ${imported} imported, ${updated} updated, ${skipped} ignored.`,
    scanFailed: "Scan failed. Check server logs.",
    startTime: "Recording Time",
    completedAt: "Completed",
    duration: "Duration",
    size: "Size",
    path: "Path",
    fileStatus: "File Status",
    noFile: "NO_FILE",
    unprotect: "Unprotect",
    protect: "Protect",
    download: "Download",
    details: "Details",
    recordingDetails: "Recording Details",
    shortRecording: "Short segment",
    recordingStatus: "Recording Status",
    localStorageStatus: "Local Status",
    file: "File",
    fileKind: "File Type",
    fileSize: "File Size",
    filePath: "File Path",
    visibleSize: "Visible Size",
    shortSegments: "Short Segments",
    protectedRecordings: "Protected Recordings",
    loadingRecordings: "Loading recordings.",
    noRecordings: "No recordings indexed yet.",
    api: "API",
    release: "Release",
    checking: "checking",
    unknown: "unknown",
    chinaTime: "China Time",
    search: "Search",
    searchPlaceholder: "Search name, room, or path",
    sortBy: "Sort by",
    sortNewest: "Recording time: newest",
    sortOldest: "Recording time: oldest",
    sortDuration: "Duration: longest",
    sortSize: "Size: largest",
    sortName: "Name: A to Z",
    sortRoom: "Room: low to high",
    sortUsername: "Username: A to Z",
    sortRole: "Role: A to Z",
    sortUpdated: "Updated: newest",
    sortRunAfter: "Run after: soonest",
    sortStatus: "Status: A to Z",
    emptyFiltered: "No matching results."
  }
} as const;

type AdminCopy = (typeof uiCopy)[Language];

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  const response = await fetch(path, {
    ...init,
    credentials: "include",
    headers
  });
  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
  }
  return (await response.json()) as T;
}

function profileToForm(profile: RecordingProfile): ProfileForm {
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
    finalize_grace_period_sec: profile.recording_settings.finalize_grace_period_sec
  };
}

function profilePayload(form: ProfileForm) {
  return {
    owner_user_id: form.owner_user_id ? Number(form.owner_user_id) : undefined,
    name: form.name,
    room_id: form.room_id,
    streamer_name: form.streamer_name,
    streamer_uid: form.streamer_uid,
    timezone: form.timezone,
    enabled: form.enabled,
    public_enabled: form.public_enabled,
    public_slug: form.public_slug,
    recording_settings: {
      auto_record: form.auto_record,
      quality: form.quality,
      record_danmaku: form.record_danmaku,
      segment_duration_sec: form.segment_duration_sec,
      finalize_grace_period_sec: form.finalize_grace_period_sec
    }
  };
}

function accountToEditForm(account: Account): AccountEditForm {
  return {
    username: account.username,
    password: "",
    enabled: account.enabled
  };
}

function accountUpdatePayload(form: AccountEditForm) {
  return {
    username: form.username,
    enabled: form.enabled,
    ...(form.password ? { password: form.password } : {})
  };
}

function hasManagerPermission(user: User | undefined, policy: ManagerPolicy | undefined, key: PolicyFlag): boolean {
  if (!user) {
    return false;
  }
  if (user.role === "SUPER_ADMIN") {
    return true;
  }
  return Boolean(policy?.[key]);
}

function includesSearch(value: string | number | undefined, search: string): boolean {
  return String(value ?? "").toLowerCase().includes(search.trim().toLowerCase());
}

function filterProfiles(items: RecordingProfile[], search: string, sort: ProfileSortKey): RecordingProfile[] {
  const filtered = items.filter((profile) => {
    if (!search.trim()) {
      return true;
    }
    return (
      includesSearch(profile.name, search) ||
      includesSearch(profile.room_id, search) ||
      includesSearch(profile.streamer_name, search) ||
      includesSearch(profile.owner_username, search)
    );
  });
  return [...filtered].sort((left, right) => {
    if (sort === "room_asc") {
      return left.room_id.localeCompare(right.room_id, "zh-CN", { numeric: true });
    }
    return left.name.localeCompare(right.name, "zh-CN");
  });
}

function filterAccounts(items: Account[], search: string, sort: AccountSortKey): Account[] {
  const filtered = items.filter((account) => {
    if (!search.trim()) {
      return true;
    }
    return includesSearch(account.username, search) || includesSearch(account.role, search);
  });
  return [...filtered].sort((left, right) => {
    if (sort === "role_asc") {
      return left.role.localeCompare(right.role) || left.username.localeCompare(right.username, "zh-CN");
    }
    return left.username.localeCompare(right.username, "zh-CN");
  });
}

function filterRecordings(items: RecordingItem[], search: string, sort: RecordingSortKey): RecordingItem[] {
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
      return (right.files?.[0]?.size_bytes ?? 0) - (left.files?.[0]?.size_bytes ?? 0);
    }
    return Date.parse(right.started_at) - Date.parse(left.started_at);
  });
}

function filterJobs(items: JobItem[], search: string, sort: JobSortKey): JobItem[] {
  const filtered = items.filter((job) => {
    if (!search.trim()) {
      return true;
    }
    return (
      includesSearch(job.type, search) ||
      includesSearch(job.status, search) ||
      includesSearch(job.resource_class, search) ||
      includesSearch(job.business_key, search) ||
      includesSearch(job.profile_name, search) ||
      includesSearch(job.owner_username, search) ||
      includesSearch(job.last_error, search)
    );
  });
  return [...filtered].sort((left, right) => {
    if (sort === "run_after_asc") {
      return Date.parse(left.run_after) - Date.parse(right.run_after);
    }
    if (sort === "status_asc") {
      return left.status.localeCompare(right.status) || Date.parse(right.updated_at) - Date.parse(left.updated_at);
    }
    return Date.parse(right.updated_at) - Date.parse(left.updated_at);
  });
}

export function AdminDashboard() {
  const queryClient = useQueryClient();
  const [language, setLanguage] = useState<Language>("zh");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [activePage, setActivePage] = useState<AdminPage>("overview");
  const [profileSearch, setProfileSearch] = useState("");
  const [profileSort, setProfileSort] = useState<ProfileSortKey>("name_asc");
  const [recordingSearch, setRecordingSearch] = useState("");
  const [recordingSort, setRecordingSort] = useState<RecordingSortKey>("started_desc");
  const [accountSearch, setAccountSearch] = useState("");
  const [accountSort, setAccountSort] = useState<AccountSortKey>("username_asc");
  const [jobSearch, setJobSearch] = useState("");
  const [jobSort, setJobSort] = useState<JobSortKey>("updated_desc");
  const [selectedProfileId, setSelectedProfileId] = useState<number | null>(null);
  const [profileEditorOpen, setProfileEditorOpen] = useState(false);
  const [profileForm, setProfileForm] = useState<ProfileForm>(emptyProfileForm);
  const [selectedAccountId, setSelectedAccountId] = useState<number | null>(null);
  const [accountEditorOpen, setAccountEditorOpen] = useState(false);
  const [accountEditForm, setAccountEditForm] = useState<AccountEditForm>(emptyAccountEditForm);
  const [storageForm, setStorageForm] = useState({
    maxRecordingGB: 0,
    minFreeGB: 0,
    emergencyFreeGB: 0,
    cleanupTargetPercent: 85
  });
  const [accountForm, setAccountForm] = useState<AccountForm>({
    ...emptyAccountForm,
    policy: { ...defaultManagerPolicy }
  });

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false
  });
  const user = meQuery.data?.user;
  const ownPolicy = meQuery.data?.policy;
  const canManageSystemSettings = user?.role === "SUPER_ADMIN";
  const canEditRecordingProfiles = hasManagerPermission(user, ownPolicy, "can_edit_recording_profile");
  const canManageLocalFiles = hasManagerPermission(user, ownPolicy, "can_manage_local_files");
  const canScanLocalFiles = Boolean(canManageSystemSettings);
  const ui = uiCopy[language];

  const healthQuery = useQuery({
    queryKey: ["system-health"],
    queryFn: () => requestJson<HealthResponse>("/api/v1/system/health"),
    retry: false,
    refetchInterval: 10000
  });

  const profilesQuery = useQuery({
    queryKey: ["recording-profiles"],
    queryFn: () => requestJson<ProfileListResponse>("/api/v1/recording-profiles"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 10000
  });

  const recordingsQuery = useQuery({
    queryKey: ["recordings"],
    queryFn: () => requestJson<RecordingListResponse>("/api/v1/recordings"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 15000
  });

  const jobsQuery = useQuery({
    queryKey: ["jobs"],
    queryFn: () => requestJson<JobListResponse>("/api/v1/jobs?limit=100"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 5000
  });

  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: () => requestJson<AccountListResponse>("/api/v1/accounts"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000
  });

  const localStorageQuery = useQuery({
    queryKey: ["local-storage"],
    queryFn: () => requestJson<LocalStorageStatus>("/api/v1/storage/local"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000
  });

  const cleanupCandidatesQuery = useQuery({
    queryKey: ["cleanup-candidates"],
    queryFn: () => requestJson<CleanupCandidateListResponse>("/api/v1/storage/local/cleanup-candidates?limit=5"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000
  });

  const profiles = profilesQuery.data?.items ?? [];
  const profileTotal = profilesQuery.data?.total ?? profiles.length;
  const selectedProfile = profiles.find((profile) => profile.id === selectedProfileId);
  const recordings = recordingsQuery.data?.items ?? [];
  const recordingTotal = recordingsQuery.data?.total ?? recordings.length;
  const jobs = jobsQuery.data?.items ?? [];
  const jobTotal = jobsQuery.data?.total ?? jobs.length;
  const accounts = accountsQuery.data?.items ?? [];
  const accountTotal = accountsQuery.data?.total ?? accounts.length;
  const visibleProfiles = filterProfiles(profiles, profileSearch, profileSort);
  const visibleRecordings = filterRecordings(recordings, recordingSearch, recordingSort);
  const visibleJobs = filterJobs(jobs, jobSearch, jobSort);
  const visibleAccounts = filterAccounts(accounts, accountSearch, accountSort);
  const selectedAccount = accounts.find((account) => account.id === selectedAccountId);

  useEffect(() => {
    if (activePage === "system" && user && !canManageSystemSettings) {
      setActivePage("overview");
    }
    if (activePage === "accounts" && user && !canManageSystemSettings) {
      setActivePage("overview");
    }
  }, [activePage, canManageSystemSettings, user]);

  useEffect(() => {
    const selected = profiles.find((profile) => profile.id === selectedProfileId);
    if (selected) {
      setProfileForm(profileToForm(selected));
    }
  }, [profiles, selectedProfileId]);

  useEffect(() => {
    const selected = accounts.find((account) => account.id === selectedAccountId);
    if (selected) {
      setAccountEditForm(accountToEditForm(selected));
    }
  }, [accounts, selectedAccountId]);

  useEffect(() => {
    const settings = localStorageQuery.data?.settings;
    if (!settings) {
      return;
    }
    setStorageForm({
      maxRecordingGB: bytesToGB(settings.max_recording_bytes),
      minFreeGB: bytesToGB(settings.min_system_free_bytes),
      emergencyFreeGB: bytesToGB(settings.absolute_emergency_free_bytes),
      cleanupTargetPercent: Math.round(settings.cleanup_target_ratio * 100)
    });
  }, [
    localStorageQuery.data?.settings?.absolute_emergency_free_bytes,
    localStorageQuery.data?.settings?.cleanup_target_ratio,
    localStorageQuery.data?.settings?.max_recording_bytes,
    localStorageQuery.data?.settings?.min_system_free_bytes
  ]);

  const loginMutation = useMutation({
    mutationFn: () =>
      requestJson<MeResponse>("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password })
      }),
    onSuccess: () => {
      setPassword("");
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  });

  const logoutMutation = useMutation({
    mutationFn: () => requestJson<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
    onSuccess: () => {
      setActivePage("overview");
      setSelectedProfileId(null);
      setProfileEditorOpen(false);
      setProfileForm(emptyProfileForm);
      setSelectedAccountId(null);
      setAccountEditorOpen(false);
      setAccountEditForm(emptyAccountEditForm);
      queryClient.removeQueries({ queryKey: ["recording-profiles"] });
      queryClient.removeQueries({ queryKey: ["recordings"] });
      queryClient.removeQueries({ queryKey: ["jobs"] });
      queryClient.removeQueries({ queryKey: ["accounts"] });
      queryClient.removeQueries({ queryKey: ["local-storage"] });
      queryClient.removeQueries({ queryKey: ["cleanup-candidates"] });
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  });

  const saveProfileMutation = useMutation({
    mutationFn: async () => {
      if (selectedProfileId) {
        const updated = await requestJson<RecordingProfile>(
          `/api/v1/recording-profiles/${selectedProfileId}`,
          {
            method: "PATCH",
            body: JSON.stringify(profilePayload(profileForm))
          }
        );
        await requestJson<RecordingSettings>(
          `/api/v1/recording-profiles/${selectedProfileId}/recording-settings`,
          {
            method: "PUT",
            body: JSON.stringify(profilePayload(profileForm).recording_settings)
          }
        );
        return updated;
      }
      return requestJson<RecordingProfile>("/api/v1/recording-profiles", {
        method: "POST",
        body: JSON.stringify(profilePayload(profileForm))
      });
    },
    onSuccess: (profile) => {
      setSelectedProfileId(profile.id);
      setProfileEditorOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
    }
  });

  const reconcileMutation = useMutation({
    mutationFn: () =>
      requestJson<ReconcileResult>("/api/v1/recording-files/reconcile", {
        method: "POST",
        body: "{}"
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["recordings"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    }
  });

  const protectRecordingMutation = useMutation({
    mutationFn: (request: { id: number; protected: boolean }) =>
      requestJson<RecordingItem>(
        `/api/v1/recordings/${request.id}/actions/${request.protected ? "protect-local" : "unprotect-local"}`,
        {
          method: "POST",
          body: "{}"
        }
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["recordings"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    }
  });

  const saveStorageSettingsMutation = useMutation({
    mutationFn: () =>
      requestJson<LocalStorageSettings>("/api/v1/storage/local/settings", {
        method: "PUT",
        body: JSON.stringify({
          max_recording_bytes: gbToBytes(storageForm.maxRecordingGB),
          min_system_free_bytes: gbToBytes(storageForm.minFreeGB),
          cleanup_target_ratio: storageForm.cleanupTargetPercent / 100,
          absolute_emergency_free_bytes: gbToBytes(storageForm.emergencyFreeGB)
        })
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    }
  });

  const cleanupMutation = useMutation({
    mutationFn: () =>
      requestJson<CleanupRunResult>("/api/v1/storage/local/actions/cleanup", {
        method: "POST",
        body: JSON.stringify({ max_recordings: Math.max(1, Math.min(5, cleanupCandidatesQuery.data?.items?.length ?? 5)) })
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["recordings"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    }
  });

  const retryJobMutation = useMutation({
    mutationFn: (jobId: number) =>
      requestJson<JobItem>(`/api/v1/jobs/${jobId}/actions/retry`, {
        method: "POST",
        body: "{}"
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    }
  });

  const cancelJobMutation = useMutation({
    mutationFn: (jobId: number) =>
      requestJson<JobItem>(`/api/v1/jobs/${jobId}/actions/cancel`, {
        method: "POST",
        body: "{}"
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    }
  });

  const archiveProfileMutation = useMutation({
    mutationFn: (profileId: number) =>
      requestJson<RecordingProfile>(`/api/v1/recording-profiles/${profileId}`, {
        method: "PATCH",
        body: JSON.stringify({ archived: true })
      }),
    onSuccess: () => {
      setSelectedProfileId(null);
      setProfileEditorOpen(false);
      setProfileForm(emptyProfileForm);
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
    }
  });

  const restoreProfileMutation = useMutation({
    mutationFn: (profileId: number) =>
      requestJson<RecordingProfile>(`/api/v1/recording-profiles/${profileId}`, {
        method: "PATCH",
        body: JSON.stringify({ archived: false, enabled: true })
      }),
    onSuccess: (profile) => {
      setSelectedProfileId(profile.id);
      setProfileEditorOpen(false);
      setProfileForm(profileToForm(profile));
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
    }
  });

  const createAccountMutation = useMutation({
    mutationFn: () =>
      requestJson<Account>("/api/v1/accounts", {
        method: "POST",
        body: JSON.stringify({
          username: accountForm.username,
          password: accountForm.password,
          role: "MANAGER",
          enabled: accountForm.enabled,
          policy: accountForm.policy
        })
      }),
    onSuccess: () => {
      setAccountForm({ ...emptyAccountForm, policy: { ...defaultManagerPolicy } });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    }
  });

  const updateAccountMutation = useMutation({
    mutationFn: (request: { accountId: number; payload: ReturnType<typeof accountUpdatePayload> | { enabled: boolean } }) =>
      requestJson<Account>(`/api/v1/accounts/${request.accountId}`, {
        method: "PATCH",
        body: JSON.stringify(request.payload)
      }),
    onSuccess: () => {
      setAccountEditorOpen(false);
      setSelectedAccountId(null);
      setAccountEditForm(emptyAccountEditForm);
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    }
  });

  const updatePolicyMutation = useMutation({
    mutationFn: (request: { accountId: number; policy: ManagerPolicy }) =>
      requestJson<ManagerPolicy>(`/api/v1/accounts/${request.accountId}/policy`, {
        method: "PUT",
        body: JSON.stringify(request.policy)
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    }
  });

  const onLoginSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    loginMutation.mutate();
  };

  const onProfileSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    saveProfileMutation.mutate();
  };

  const onAccountSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createAccountMutation.mutate();
  };

  const onAccountEditSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedAccountId) {
      return;
    }
    updateAccountMutation.mutate({
      accountId: selectedAccountId,
      payload: accountUpdatePayload(accountEditForm)
    });
  };

  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <header className="border-b border-border pb-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <p className="text-sm font-medium text-accent">{ui.appName}</p>
              <h1 className="mt-2 text-3xl font-semibold tracking-normal">{ui.title}</h1>
            </div>
            {user ? (
              <AccountMenu
                language={language}
                labels={ui}
                onAccount={() => setActivePage("me")}
                logoutPending={logoutMutation.isPending}
                user={user}
                onLogout={() => logoutMutation.mutate()}
                onLanguageChange={setLanguage}
              />
            ) : (
              <LanguageControl language={language} labels={ui} onLanguageChange={setLanguage} />
            )}
          </div>
          {user ? (
            <AdminNav
              activePage={activePage}
              canManageSystemSettings={Boolean(canManageSystemSettings)}
              labels={ui}
              onChange={setActivePage}
            />
          ) : null}
        </header>

        {user ? null : (
          <SessionPanel
            loginError={loginMutation.isError}
            labels={ui}
            logoutPending={logoutMutation.isPending}
            password={password}
            username={username}
            user={user}
            onLoginSubmit={onLoginSubmit}
            onLogout={() => logoutMutation.mutate()}
            onPasswordChange={setPassword}
            onUsernameChange={setUsername}
          />
        )}

        {user ? (
          <>
            {activePage === "overview" ? <OverviewPanel statusRows={ui.statusRows} /> : null}

            {activePage === "me" ? (
              <MyAccountPanel
                canManageSystemSettings={Boolean(canManageSystemSettings)}
                labels={ui}
                policy={ownPolicy}
                user={user}
              />
            ) : null}

            {activePage === "profiles" ? (
              <ProfileListPanel
                labels={ui}
                canEdit={canEditRecordingProfiles}
                profiles={visibleProfiles}
                search={profileSearch}
                selectedProfileId={selectedProfileId}
                sort={profileSort}
                total={profileTotal}
                visibleTotal={visibleProfiles.length}
                showOwner={Boolean(canManageSystemSettings)}
                onCreate={() => {
                  if (!canEditRecordingProfiles) {
                    return;
                  }
                  setSelectedProfileId(null);
                  setProfileForm({ ...emptyProfileForm, owner_user_id: String(user.id) });
                  setProfileEditorOpen(true);
                }}
                onSelect={(profile) => {
                  if (!canEditRecordingProfiles) {
                    return;
                  }
                  setSelectedProfileId(profile.id);
                  setProfileForm(profileToForm(profile));
                  setProfileEditorOpen(true);
                }}
                onSearchChange={setProfileSearch}
                onSortChange={setProfileSort}
              />
            ) : null}

            {activePage === "accounts" && canManageSystemSettings ? (
              <AccountsPanel
                accountForm={accountForm}
                accounts={visibleAccounts}
                createError={createAccountMutation.isError}
                createPending={createAccountMutation.isPending}
                currentUserId={user.id}
                labels={ui}
                policyPending={updatePolicyMutation.isPending}
                search={accountSearch}
                sort={accountSort}
                total={accountTotal}
                visibleTotal={visibleAccounts.length}
                updatePending={updateAccountMutation.isPending}
                onAccountFormChange={setAccountForm}
                onCreate={onAccountSubmit}
                onSearchChange={setAccountSearch}
                onSortChange={setAccountSort}
                onEdit={(account) => {
                  setSelectedAccountId(account.id);
                  setAccountEditForm(accountToEditForm(account));
                  setAccountEditorOpen(true);
                }}
                onToggleEnabled={(account) =>
                  updateAccountMutation.mutate({ accountId: account.id, payload: { enabled: !account.enabled } })
                }
                onUpdatePolicy={(account, policy) =>
                  updatePolicyMutation.mutate({ accountId: account.id, policy })
                }
              />
            ) : null}

            {accountEditorOpen && selectedAccount ? (
              <AccountEditorDialog
                account={selectedAccount}
                currentUserId={user.id}
                form={accountEditForm}
                isSaving={updateAccountMutation.isPending}
                labels={ui}
                saveError={updateAccountMutation.isError}
                onCancel={() => {
                  setAccountEditorOpen(false);
                  setSelectedAccountId(null);
                  setAccountEditForm(emptyAccountEditForm);
                }}
                onChange={setAccountEditForm}
                onSubmit={onAccountEditSubmit}
              />
            ) : null}

            {profileEditorOpen ? (
              <ProfileEditorDialog
                archivePending={archiveProfileMutation.isPending}
                form={profileForm}
                isEditing={Boolean(selectedProfileId)}
                isSaving={saveProfileMutation.isPending}
                labels={ui}
                profile={selectedProfile}
                restorePending={restoreProfileMutation.isPending}
                saveError={saveProfileMutation.isError}
                ownerAccounts={accounts}
                showOwner={Boolean(canManageSystemSettings)}
                onArchive={(profileId) => archiveProfileMutation.mutate(profileId)}
                onCancel={() => {
                  setProfileEditorOpen(false);
                  setSelectedProfileId(null);
                  setProfileForm(emptyProfileForm);
                }}
                onChange={setProfileForm}
                onRestore={(profileId) => restoreProfileMutation.mutate(profileId)}
                onSubmit={onProfileSubmit}
              />
            ) : null}

            {activePage === "system" && canManageSystemSettings ? (
              <StoragePanel
                candidates={cleanupCandidatesQuery.data?.items ?? []}
                previewReclaimableBytes={cleanupCandidatesQuery.data?.preview_reclaimable_bytes ?? 0}
                form={storageForm}
                cleanupError={cleanupMutation.isError}
                cleanupPending={cleanupMutation.isPending}
                cleanupResult={cleanupMutation.data}
                isLoading={localStorageQuery.isLoading}
                isSaving={saveStorageSettingsMutation.isPending}
                labels={ui}
                saveError={saveStorageSettingsMutation.isError}
                status={localStorageQuery.data}
                onFormChange={setStorageForm}
                onRunCleanup={() => {
                  if (window.confirm(ui.cleanupConfirm)) {
                    cleanupMutation.mutate();
                  }
                }}
                onSave={() => saveStorageSettingsMutation.mutate()}
              />
            ) : null}

            {activePage === "recordings" ? (
              <RecordingsPanel
                isLoading={recordingsQuery.isLoading}
                labels={ui}
                canManageLocalFiles={canManageLocalFiles}
                canScanLocalFiles={canScanLocalFiles}
                protectPending={protectRecordingMutation.isPending}
                reconcileError={reconcileMutation.isError}
                reconcilePending={reconcileMutation.isPending}
                reconcileResult={reconcileMutation.data}
                recordings={visibleRecordings}
                search={recordingSearch}
                sort={recordingSort}
                total={recordingTotal}
                visibleTotal={visibleRecordings.length}
                onReconcile={() => reconcileMutation.mutate()}
                onSearchChange={setRecordingSearch}
                onSortChange={setRecordingSort}
                onToggleProtect={(recording) =>
                  protectRecordingMutation.mutate({ id: recording.id, protected: !recording.local_protected })
                }
              />
            ) : null}

            {activePage === "jobs" ? (
              <JobsPanel
                cancelPending={cancelJobMutation.isPending}
                isLoading={jobsQuery.isLoading}
                jobs={visibleJobs}
                labels={ui}
                loadError={jobsQuery.isError}
                retryPending={retryJobMutation.isPending}
                search={jobSearch}
                sort={jobSort}
                total={jobTotal}
                visibleTotal={visibleJobs.length}
                onCancel={(job) => cancelJobMutation.mutate(job.id)}
                onRetry={(job) => retryJobMutation.mutate(job.id)}
                onSearchChange={setJobSearch}
                onSortChange={setJobSort}
              />
            ) : null}
          </>
        ) : null}

        <section className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-border pt-4 text-sm text-muted">
          <span>{ui.api}: {healthQuery.data?.status ?? ui.checking}</span>
          <span>{ui.release}: {healthQuery.data?.release_sha ?? ui.unknown}</span>
        </section>
      </div>
    </main>
  );
}

function AdminNav(props: {
  activePage: AdminPage;
  canManageSystemSettings: boolean;
  labels: AdminCopy;
  onChange: (page: AdminPage) => void;
}) {
  const items: Array<{ page: AdminPage; label: string; icon: typeof Activity }> = [
    { page: "overview", label: props.labels.nav.overview, icon: LayoutDashboard },
    { page: "profiles", label: props.labels.nav.profiles, icon: Activity },
    { page: "recordings", label: props.labels.nav.recordings, icon: FileVideo },
    { page: "jobs", label: props.labels.nav.jobs, icon: RefreshCw }
  ];

  if (props.canManageSystemSettings) {
    items.push({ page: "accounts", label: props.labels.nav.accounts, icon: Users });
    items.push({ page: "system", label: props.labels.nav.system, icon: Settings });
  }

  return (
    <nav className="mt-5 flex flex-wrap gap-2" aria-label={props.labels.navLabel}>
      {items.map(({ page, label, icon: Icon }) => {
        const isActive = props.activePage === page;
        return (
          <button
            key={page}
            className={`inline-flex h-10 items-center justify-center gap-2 rounded-md border px-3 text-sm font-medium shadow-sm ${
              isActive
                ? "border-accent bg-accent text-white"
                : "border-border bg-panel text-ink hover:border-accent hover:text-accent"
            }`}
            type="button"
            onClick={() => props.onChange(page)}
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
            {label}
          </button>
        );
      })}
    </nav>
  );
}

function AccountMenu(props: {
  language: Language;
  labels: AdminCopy;
  logoutPending: boolean;
  user: User;
  onAccount: () => void;
  onLanguageChange: (language: Language) => void;
  onLogout: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-panel px-3 py-2 shadow-sm">
      <UserCircle className="h-5 w-5 text-accent" aria-hidden="true" />
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{props.user.username}</p>
        <p className="text-xs text-muted">{props.user.role}</p>
      </div>
      <LanguageControl language={props.language} labels={props.labels} onLanguageChange={props.onLanguageChange} />
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-medium text-ink hover:border-accent hover:text-accent"
        type="button"
        onClick={props.onAccount}
      >
        <UserCircle className="h-4 w-4" aria-hidden="true" />
        {props.labels.myAccount}
      </button>
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-ink px-3 text-sm font-medium text-white disabled:opacity-60"
        disabled={props.logoutPending}
        type="button"
        onClick={props.onLogout}
      >
        <LogOut className="h-4 w-4" aria-hidden="true" />
        {props.labels.signOut}
      </button>
    </div>
  );
}

function LanguageControl(props: {
  language: Language;
  labels: AdminCopy;
  onLanguageChange: (language: Language) => void;
}) {
  return (
    <label className="flex items-center gap-2 rounded-md border border-border bg-panel px-3 py-2 text-xs font-medium text-muted shadow-sm">
      {props.labels.language}
      <select
        className="h-9 rounded-md border border-border bg-white px-2 text-sm font-medium text-ink outline-none focus:border-accent"
        value={props.language}
        onChange={(event) => props.onLanguageChange(event.target.value as Language)}
      >
        <option value="zh">{props.labels.chinese}</option>
        <option value="en">{props.labels.english}</option>
      </select>
    </label>
  );
}

function MyAccountPanel(props: {
  canManageSystemSettings: boolean;
  labels: AdminCopy;
  policy?: ManagerPolicy;
  user: User;
}) {
  return (
    <section className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.myAccount}</h2>
        <div className="mt-4 grid gap-3">
          <Metric label={props.labels.username} value={props.user.username} />
          <Metric label={props.labels.role} value={props.user.role} />
          <Metric label={props.labels.status} value={props.user.enabled ? props.labels.enabled : props.labels.disabled} />
        </div>
      </section>

      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.access}</h2>
        {props.canManageSystemSettings ? (
          <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <Metric label={props.labels.systemSettings} value={props.labels.allowed} />
            <Metric label={props.labels.accounts} value={props.labels.allowed} />
            <Metric label={props.labels.profiles} value={props.labels.allOwners} />
          </div>
        ) : props.policy ? (
          <div className="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-3">
            <PermissionBadge labels={props.labels} label={props.labels.recordingProfiles} enabled={props.policy.can_edit_recording_profile} />
            <PermissionBadge labels={props.labels} label={props.labels.bilibiliConfig} enabled={props.policy.can_edit_bilibili_module} />
            <PermissionBadge labels={props.labels} label={props.labels.cosConfig} enabled={props.policy.can_edit_cos_module} />
            <PermissionBadge labels={props.labels} label={props.labels.neteaseConfig} enabled={props.policy.can_edit_netease_module} />
            <PermissionBadge labels={props.labels} label={props.labels.localFiles} enabled={props.policy.can_manage_local_files} />
          </div>
        ) : (
          <p className="mt-4 text-sm text-muted">{props.labels.policyUnavailable}</p>
        )}
      </section>
    </section>
  );
}

function PermissionBadge(props: { enabled: boolean; label: string; labels: AdminCopy }) {
  return (
    <div className="rounded-md border border-border bg-white px-3 py-2">
      <p className="text-xs uppercase text-muted">{props.label}</p>
      <p className={`mt-1 text-sm font-semibold ${props.enabled ? "text-accent" : "text-muted"}`}>
        {props.enabled ? props.labels.allowed : props.labels.blocked}
      </p>
    </div>
  );
}

function OverviewPanel(props: { statusRows: AdminCopy["statusRows"] }) {
  return (
    <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {props.statusRows.map(({ label, value, icon: Icon }) => (
        <article key={label} className="rounded-md border border-border bg-panel p-4 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold">{label}</h2>
              <p className="mt-2 text-sm leading-5 text-muted">{value}</p>
            </div>
            <Icon className="h-5 w-5 shrink-0 text-accent" aria-hidden="true" />
          </div>
        </article>
      ))}
    </section>
  );
}

function SessionPanel(props: {
  labels: AdminCopy;
  loginError: boolean;
  logoutPending: boolean;
  password: string;
  username: string;
  user?: User;
  onLoginSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onLogout: () => void;
  onPasswordChange: (value: string) => void;
  onUsernameChange: (value: string) => void;
}) {
  if (props.user) {
    return (
      <aside className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.session}</h2>
        <div className="mt-4 flex flex-col gap-4">
          <div>
            <p className="text-lg font-semibold">{props.user.username}</p>
            <p className="text-sm text-muted">{props.user.role}</p>
          </div>
          <button
            className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-ink px-3 text-sm font-medium text-white disabled:opacity-60"
            disabled={props.logoutPending}
            type="button"
            onClick={props.onLogout}
          >
            <LogOut className="h-4 w-4" aria-hidden="true" />
            {props.labels.signOut}
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <h2 className="text-sm font-semibold">{props.labels.session}</h2>
      <form className="mt-4 flex flex-col gap-3" onSubmit={props.onLoginSubmit}>
        <TextField
          autoComplete="username"
          label={props.labels.username}
          value={props.username}
          onChange={props.onUsernameChange}
        />
        <TextField
          autoComplete="current-password"
          label={props.labels.password}
          type="password"
          value={props.password}
          onChange={props.onPasswordChange}
        />
        {props.loginError ? (
          <p className="text-sm text-red-700">{props.labels.loginFailed}</p>
        ) : null}
        <button className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white">
          <LogIn className="h-4 w-4" aria-hidden="true" />
          {props.labels.signIn}
        </button>
      </form>
    </aside>
  );
}

function ProfileEditorDialog(props: {
  archivePending: boolean;
  form: ProfileForm;
  isEditing: boolean;
  isSaving: boolean;
  labels: AdminCopy;
  ownerAccounts: Account[];
  profile?: RecordingProfile;
  restorePending: boolean;
  saveError: boolean;
  showOwner: boolean;
  onArchive: (profileId: number) => void;
  onCancel: () => void;
  onChange: (form: ProfileForm) => void;
  onRestore: (profileId: number) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const [confirmArchive, setConfirmArchive] = useState(false);
  const update = <K extends keyof ProfileForm>(key: K, value: ProfileForm[K]) => {
    props.onChange({ ...props.form, [key]: value });
  };
  const isArchived = Boolean(props.profile?.archived_at);
  const ownerOptions = props.ownerAccounts.filter((account) => account.enabled);
  const selectedOwnerMissing =
    props.form.owner_user_id && !ownerOptions.some((account) => String(account.id) === props.form.owner_user_id);

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/35 px-4 py-6">
      <form
        className="w-full max-w-xl rounded-md border border-border bg-panel p-4 shadow-xl"
        onSubmit={props.onSubmit}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border pb-3">
          <div>
            <h2 className="text-base font-semibold">{props.isEditing ? props.labels.editProfile : props.labels.newProfile}</h2>
            {isArchived ? <p className="mt-1 text-xs font-medium text-muted">{props.labels.archivedProfile}</p> : null}
          </div>
          <button
            className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border text-ink hover:border-accent hover:text-accent"
            type="button"
            onClick={props.onCancel}
          >
            <X className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{props.labels.close}</span>
          </button>
        </div>

        <div className="mt-4 grid gap-3">
          {props.showOwner ? (
            <label className="flex flex-col gap-1 text-sm font-medium">
              {props.labels.owner}
              <select
                className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
                value={props.form.owner_user_id}
                onChange={(event) => update("owner_user_id", event.target.value)}
              >
                {selectedOwnerMissing ? <option value={props.form.owner_user_id}>{props.labels.currentOwner}</option> : null}
                {ownerOptions.map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.username} ({account.role})
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          <TextField label={props.labels.name} value={props.form.name} onChange={(value) => update("name", value)} />
          <TextField
            label={props.labels.roomId}
            value={props.form.room_id}
            onChange={(value) => update("room_id", value)}
          />
          <TextField
            label={props.labels.streamer}
            value={props.form.streamer_name}
            onChange={(value) => update("streamer_name", value)}
          />
          <TextField
            label={props.labels.streamerUid}
            value={props.form.streamer_uid}
            onChange={(value) => update("streamer_uid", value)}
          />
          <TextField
            label={props.labels.timezone}
            value={props.form.timezone}
            onChange={(value) => update("timezone", value)}
          />
          <TextField
            label={props.labels.publicSlug}
            value={props.form.public_slug}
            onChange={(value) => update("public_slug", value)}
          />
          <SelectField label={props.labels.quality} value={props.form.quality} onChange={(value) => update("quality", value)} />
          <NumberField
            label={props.labels.segmentSeconds}
            min={60}
            value={props.form.segment_duration_sec}
            onChange={(value) => update("segment_duration_sec", value)}
          />
          <NumberField
            label={props.labels.finalizeGraceSeconds}
            min={0}
            value={props.form.finalize_grace_period_sec}
            onChange={(value) => update("finalize_grace_period_sec", value)}
          />
          <ToggleField label={props.labels.enabled} checked={props.form.enabled} onChange={(value) => update("enabled", value)} />
          <ToggleField
            label={props.labels.autoRecord}
            checked={props.form.auto_record}
            onChange={(value) => update("auto_record", value)}
          />
          <ToggleField
            label={props.labels.recordDanmaku}
            checked={props.form.record_danmaku}
            onChange={(value) => update("record_danmaku", value)}
          />
          <ToggleField
            label={props.labels.publicPage}
            checked={props.form.public_enabled}
            onChange={(value) => update("public_enabled", value)}
          />
        </div>

        {props.saveError ? (
          <p className="mt-3 text-sm text-red-700">{props.labels.profileSaveFailed}</p>
        ) : null}

        <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
          <div>
            {props.profile && isArchived ? (
              <button
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-semibold text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                disabled={props.restorePending}
                type="button"
                onClick={() => props.onRestore(props.profile!.id)}
              >
                <ArchiveRestore className="h-4 w-4" aria-hidden="true" />
                {props.labels.restoreProfile}
              </button>
            ) : null}
            {props.profile && !isArchived ? (
              <div className="flex flex-wrap items-center gap-2">
                {confirmArchive ? (
                  <>
                    <button
                      className="inline-flex h-9 items-center justify-center rounded-md bg-red-700 px-3 text-sm font-semibold text-white disabled:opacity-60"
                      disabled={props.archivePending}
                      type="button"
                      onClick={() => props.onArchive(props.profile!.id)}
                    >
                      {props.labels.confirmArchive}
                    </button>
                    <button
                      className="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm font-medium text-ink"
                      type="button"
                      onClick={() => setConfirmArchive(false)}
                    >
                      {props.labels.cancel}
                    </button>
                  </>
                ) : (
                  <button
                    className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-red-200 px-3 text-sm font-semibold text-red-700 hover:border-red-700 disabled:opacity-60"
                    disabled={props.archivePending}
                    type="button"
                    onClick={() => setConfirmArchive(true)}
                  >
                    <Archive className="h-4 w-4" aria-hidden="true" />
                    {props.labels.archiveProfile}
                  </button>
                )}
              </div>
            ) : null}
          </div>

          <div className="flex items-center gap-2">
            <button
              className="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm font-medium text-ink"
              type="button"
              onClick={props.onCancel}
            >
              {props.labels.cancel}
            </button>
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
              disabled={props.isSaving}
              type="submit"
            >
              {props.isEditing ? <Save className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
              {props.isEditing ? props.labels.save : props.labels.create}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}

function AccountsPanel(props: {
  accountForm: AccountForm;
  accounts: Account[];
  createError: boolean;
  createPending: boolean;
  currentUserId: number;
  labels: AdminCopy;
  policyPending: boolean;
  search: string;
  sort: AccountSortKey;
  total: number;
  visibleTotal: number;
  updatePending: boolean;
  onAccountFormChange: (form: AccountForm) => void;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
  onEdit: (account: Account) => void;
  onSearchChange: (value: string) => void;
  onSortChange: (value: AccountSortKey) => void;
  onToggleEnabled: (account: Account) => void;
  onUpdatePolicy: (account: Account, policy: ManagerPolicy) => void;
}) {
  const updateForm = <K extends keyof AccountForm>(key: K, value: AccountForm[K]) => {
    props.onAccountFormChange({ ...props.accountForm, [key]: value });
  };
  const updateFormPolicy = (key: PolicyFlag, value: boolean) => {
    props.onAccountFormChange({
      ...props.accountForm,
      policy: { ...props.accountForm.policy, [key]: value }
    });
  };

  return (
    <section className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
      <form className="rounded-md border border-border bg-panel p-4 shadow-sm" onSubmit={props.onCreate}>
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold">{props.labels.newManager}</h2>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.createPending}
            type="submit"
          >
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            {props.labels.create}
          </button>
        </div>
        <div className="mt-4 grid gap-3">
          <TextField
            autoComplete="username"
            label={props.labels.username}
            value={props.accountForm.username}
            onChange={(value) => updateForm("username", value)}
          />
          <TextField
            autoComplete="new-password"
            label={props.labels.initialPassword}
            type="password"
            value={props.accountForm.password}
            onChange={(value) => updateForm("password", value)}
          />
          <ToggleField
            label={props.labels.enabled}
            checked={props.accountForm.enabled}
            onChange={(value) => updateForm("enabled", value)}
          />
          <AccountPolicyFields
            labels={props.labels}
            policy={props.accountForm.policy}
            onChange={(key, value) => updateFormPolicy(key, value)}
          />
          {props.createError ? (
            <p className="text-sm text-red-700">{props.labels.accountCreationFailed}</p>
          ) : null}
        </div>
      </form>

      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold">{props.labels.accounts}</h2>
          <span className="text-sm text-muted">{props.labels.total(props.visibleTotal)} / {props.total}</span>
        </div>
        <TableToolbar
          labels={props.labels}
          search={props.search}
          sort={props.sort}
          sortOptions={[
            { value: "username_asc", label: props.labels.sortUsername },
            { value: "role_asc", label: props.labels.sortRole }
          ]}
          onSearchChange={props.onSearchChange}
          onSortChange={(value) => props.onSortChange(value as AccountSortKey)}
        />
        <div className="mt-4 overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">{props.labels.account}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.role}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.profiles}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.status}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.actions}</th>
              </tr>
            </thead>
            <tbody>
              {props.accounts.map((account) => (
                <tr key={account.id} className="bg-white align-top">
                  <td className="px-3 py-3">
                    <p className="font-semibold text-ink">{account.username}</p>
                    <p className="mt-1 text-xs text-muted">ID {account.id}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">{account.role}</td>
                  <td className="px-3 py-3 text-muted">{account.profile_count}</td>
                  <td className="px-3 py-3 text-muted">{account.enabled ? props.labels.enabled : props.labels.disabled}</td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
                      <button
                        className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                        type="button"
                        onClick={() => props.onEdit(account)}
                      >
                        {props.labels.edit}
                      </button>
                      <button
                        className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                        disabled={props.updatePending || account.id === props.currentUserId}
                        type="button"
                        onClick={() => props.onToggleEnabled(account)}
                      >
                        {account.enabled ? props.labels.disable : props.labels.enable}
                      </button>
                      {account.policy ? (
                        <div className="grid gap-2 pt-1">
                          <AccountPolicyFields
                            compact
                            labels={props.labels}
                            policy={account.policy}
                            onChange={(key, value) =>
                              props.onUpdatePolicy(account, { ...account.policy!, [key]: value })
                            }
                            disabled={props.policyPending}
                          />
                        </div>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
              {props.accounts.length === 0 ? (
                <tr>
                  <td className="px-3 py-8 text-center text-muted" colSpan={5}>
                    {props.accounts.length === 0 && props.search ? props.labels.emptyFiltered : props.labels.noAccounts}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
    </section>
  );
}

function AccountPolicyFields(props: {
  compact?: boolean;
  disabled?: boolean;
  labels: AdminCopy;
  policy: ManagerPolicy;
  onChange: (key: PolicyFlag, value: boolean) => void;
}) {
  const fields: Array<{ key: PolicyFlag; label: string }> = [
    { key: "can_edit_recording_profile", label: props.labels.editProfiles },
    { key: "can_edit_bilibili_module", label: props.labels.bilibiliConfig },
    { key: "can_edit_cos_module", label: props.labels.cosConfig },
    { key: "can_edit_netease_module", label: props.labels.neteaseConfig },
    { key: "can_manage_local_files", label: props.labels.localFiles }
  ];

  return (
    <div className={props.compact ? "grid gap-1" : "grid gap-2"}>
      {fields.map((field) => (
        <ToggleField
          key={field.key}
          checked={Boolean(props.policy[field.key])}
          disabled={props.disabled}
          label={field.label}
          onChange={(value) => props.onChange(field.key, value)}
        />
      ))}
    </div>
  );
}

function AccountEditorDialog(props: {
  account: Account;
  currentUserId: number;
  form: AccountEditForm;
  isSaving: boolean;
  labels: AdminCopy;
  saveError: boolean;
  onCancel: () => void;
  onChange: (form: AccountEditForm) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const update = <K extends keyof AccountEditForm>(key: K, value: AccountEditForm[K]) => {
    props.onChange({ ...props.form, [key]: value });
  };
  const isCurrentUser = props.account.id === props.currentUserId;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/35 px-4 py-6">
      <form
        className="w-full max-w-lg rounded-md border border-border bg-panel p-4 shadow-xl"
        onSubmit={props.onSubmit}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border pb-3">
          <div>
            <h2 className="text-base font-semibold">{props.labels.editAccount}</h2>
            <p className="mt-1 text-xs text-muted">
              ID {props.account.id} · {isCurrentUser ? props.labels.currentAccount : props.account.role}
            </p>
          </div>
          <button
            className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border text-ink hover:border-accent hover:text-accent"
            type="button"
            onClick={props.onCancel}
          >
            <X className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{props.labels.close}</span>
          </button>
        </div>

        <div className="mt-4 grid gap-3">
          <TextField
            autoComplete="username"
            label={props.labels.username}
            value={props.form.username}
            onChange={(value) => update("username", value)}
          />
          <TextField
            autoComplete="new-password"
            label={props.labels.newPassword}
            type="password"
            value={props.form.password}
            onChange={(value) => update("password", value)}
          />
          <p className="-mt-2 text-xs text-muted">{props.labels.newPasswordHint}</p>
          <ToggleField
            disabled={isCurrentUser}
            label={props.labels.enabled}
            checked={props.form.enabled}
            onChange={(value) => update("enabled", value)}
          />
        </div>

        {props.saveError ? (
          <p className="mt-3 text-sm text-red-700">{props.labels.accountSaveFailed}</p>
        ) : null}

        <div className="mt-5 flex justify-end gap-2 border-t border-border pt-4">
          <button
            className="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm font-medium text-ink"
            type="button"
            onClick={props.onCancel}
          >
            {props.labels.cancel}
          </button>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.isSaving}
            type="submit"
          >
            <Save className="h-4 w-4" aria-hidden="true" />
            {props.labels.saveAccount}
          </button>
        </div>
      </form>
    </div>
  );
}

function ProfileListPanel(props: {
  canEdit: boolean;
  labels: AdminCopy;
  profiles: RecordingProfile[];
  search: string;
  selectedProfileId: number | null;
  showOwner: boolean;
  sort: ProfileSortKey;
  total: number;
  visibleTotal: number;
  onCreate: () => void;
  onSearchChange: (value: string) => void;
  onSelect: (profile: RecordingProfile) => void;
  onSortChange: (value: ProfileSortKey) => void;
}) {
  return (
    <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">{props.labels.recordingProfiles}</h2>
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted">{props.labels.total(props.visibleTotal)} / {props.total}</span>
          {props.canEdit ? (
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white"
              type="button"
              onClick={props.onCreate}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
              {props.labels.new}
            </button>
          ) : null}
        </div>
      </div>
      <TableToolbar
        labels={props.labels}
        search={props.search}
        sort={props.sort}
        sortOptions={[
          { value: "name_asc", label: props.labels.sortName },
          { value: "room_asc", label: props.labels.sortRoom }
        ]}
        onSearchChange={props.onSearchChange}
        onSortChange={(value) => props.onSortChange(value as ProfileSortKey)}
      />
      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.name}</th>
              {props.showOwner ? <th className="px-3 py-2 font-semibold">{props.labels.ownerColumn}</th> : null}
              <th className="px-3 py-2 font-semibold">{props.labels.room}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.runtime}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.sync}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.actions}</th>
            </tr>
          </thead>
          <tbody>
            {props.profiles.map((profile) => (
              <tr
                key={profile.id}
                className={profile.id === props.selectedProfileId ? "bg-[#f4f7f1]" : "bg-white"}
              >
                <td className="px-3 py-3">
                  <button
                    className={`font-semibold ${props.canEdit ? "text-ink hover:text-accent" : "cursor-default text-ink"}`}
                    type="button"
                    onClick={() => {
                      if (props.canEdit) {
                        props.onSelect(profile);
                      }
                    }}
                  >
                    {profile.name}
                  </button>
                  <p className="mt-1 text-xs text-muted">{profile.streamer_name}</p>
                </td>
                {props.showOwner ? (
                  <td className="px-3 py-3 text-muted">{profile.owner_username || profile.owner_user_id}</td>
                ) : null}
                <td className="px-3 py-3 text-muted">{profile.room_id}</td>
                <td className="px-3 py-3 text-muted">{profile.runtime.recorder_status}</td>
                <td className="px-3 py-3 text-muted">{profile.runtime.sync_status}</td>
                <td className="px-3 py-3">
                  {!props.canEdit ? (
                    <span className="text-xs text-muted">{props.labels.noAction}</span>
                  ) : profile.archived_at ? (
                    <span className="rounded-md border border-border px-2 py-1 text-xs font-medium text-muted">
                      {props.labels.archived}
                    </span>
                  ) : (
                    <button
                      className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                      type="button"
                      onClick={() => props.onSelect(profile)}
                    >
                      {props.labels.edit}
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {props.profiles.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={props.showOwner ? 6 : 5}>
                  {props.profiles.length === 0 && props.search ? props.labels.emptyFiltered : props.labels.noProfiles}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function StoragePanel(props: {
  candidates: CleanupCandidate[];
  cleanupError: boolean;
  cleanupPending: boolean;
  cleanupResult?: CleanupRunResult;
  form: {
    maxRecordingGB: number;
    minFreeGB: number;
    emergencyFreeGB: number;
    cleanupTargetPercent: number;
  };
  isLoading: boolean;
  isSaving: boolean;
  labels: AdminCopy;
  previewReclaimableBytes: number;
  saveError: boolean;
  status?: LocalStorageStatus;
  onFormChange: (form: {
    maxRecordingGB: number;
    minFreeGB: number;
    emergencyFreeGB: number;
    cleanupTargetPercent: number;
  }) => void;
  onRunCleanup: () => void;
  onSave: () => void;
}) {
  const usedPercent =
    props.status && props.status.disk_total_bytes > 0
      ? Math.round(((props.status.disk_total_bytes - props.status.disk_available_bytes) / props.status.disk_total_bytes) * 100)
      : 0;
  const update = <K extends keyof typeof props.form>(key: K, value: (typeof props.form)[K]) => {
    props.onFormChange({ ...props.form, [key]: value });
  };

  return (
    <section id="storage" className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{props.labels.localStorage}</h2>
          <p className="mt-1 text-sm text-muted">{props.status?.data_root ?? props.labels.checkingStorage}</p>
        </div>
        <HardDrive className="h-5 w-5 shrink-0 text-accent" aria-hidden="true" />
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <Metric label={props.labels.indexedVideos} value={props.isLoading ? "..." : String(props.status?.indexed_video_files ?? 0)} />
        <Metric label={props.labels.indexedSize} value={formatBytes(props.status?.indexed_video_bytes ?? 0)} />
        <Metric label={props.labels.diskAvailable} value={formatBytes(props.status?.disk_available_bytes ?? 0)} />
        <Metric label={props.labels.protected} value={String(props.status?.protected_recordings ?? 0)} />
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <Metric label={props.labels.health} value={props.status?.health ?? props.labels.checking} />
        <Metric label={props.labels.needReclaim} value={formatBytes(props.status?.need_reclaim_bytes ?? 0)} />
        <Metric label={props.labels.previewReclaimable} value={formatBytes(props.previewReclaimableBytes)} />
      </div>

      <div className="mt-4 h-2 overflow-hidden rounded-full bg-[#e6ebe4]">
        <div className="h-full bg-accent" style={{ width: `${Math.min(100, Math.max(0, usedPercent))}%` }} />
      </div>
      <p className="mt-2 text-xs text-muted">
        {props.labels.diskSummary(
          usedPercent,
          formatBytes(props.status?.disk_total_bytes ?? 0),
          props.status?.completed_recordings ?? 0,
          Boolean(props.status?.settings_configured)
        )}
      </p>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">{props.labels.storageSettings}</h3>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.isSaving}
            type="button"
            onClick={props.onSave}
          >
            <Save className="h-4 w-4" aria-hidden="true" />
            {props.labels.save}
          </button>
        </div>
        <div className="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <NumberField
            label={props.labels.maxRecordingGB}
            min={1}
            value={props.form.maxRecordingGB}
            onChange={(value) => update("maxRecordingGB", value)}
          />
          <NumberField
            label={props.labels.minFreeGB}
            min={1}
            value={props.form.minFreeGB}
            onChange={(value) => update("minFreeGB", value)}
          />
          <NumberField
            label={props.labels.emergencyFreeGB}
            min={1}
            value={props.form.emergencyFreeGB}
            onChange={(value) => update("emergencyFreeGB", value)}
          />
          <NumberField
            label={props.labels.cleanupTargetPercent}
            max={99}
            min={1}
            value={props.form.cleanupTargetPercent}
            onChange={(value) => update("cleanupTargetPercent", value)}
          />
        </div>
        {props.saveError ? (
          <p className="mt-3 text-sm text-red-700">{props.labels.storageSaveFailed}</p>
        ) : null}
      </div>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">{props.labels.cleanupPreview}</h3>
            <span className="text-xs text-muted">{props.labels.oldestUnprotected}</span>
          </div>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-red-300 px-3 text-sm font-semibold text-red-700 hover:border-red-500 disabled:opacity-50"
            disabled={props.cleanupPending || props.candidates.length === 0 || (props.status?.need_reclaim_bytes ?? 0) <= 0}
            type="button"
            onClick={props.onRunCleanup}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            {props.labels.runCleanup}
          </button>
        </div>
        {props.cleanupResult ? (
          <p className="mt-3 text-sm text-muted">
            {props.labels.cleanupResult(
              props.cleanupResult.deleted_recordings,
              props.cleanupResult.deleted_files,
              formatBytes(props.cleanupResult.reclaimed_bytes),
              props.cleanupResult.skipped_recordings
            )}
          </p>
        ) : null}
        {props.cleanupError ? (
          <p className="mt-3 text-sm text-red-700">{props.labels.cleanupFailed}</p>
        ) : null}
        <div className="mt-3 overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">{props.labels.recording}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.closed}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.files}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.reclaimable}</th>
              </tr>
            </thead>
            <tbody>
              {props.candidates.map((candidate) => (
                <tr key={candidate.recording_id} className="bg-white">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">{candidate.title || candidate.streamer_name || props.labels.untitled}</p>
                    <p className="mt-1 text-xs text-muted">
                      {candidate.profile_name} - {candidate.room_id}
                    </p>
                  </td>
                  <td className="px-3 py-3 text-xs text-muted">{formatDateTime(candidate.completed_at || "", props.labels)}</td>
                  <td className="px-3 py-3 text-muted">{candidate.file_count}</td>
                  <td className="px-3 py-3 text-muted">{formatBytes(candidate.reclaimable_bytes)}</td>
                </tr>
              ))}
              {props.candidates.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-muted" colSpan={4}>
                    {props.labels.noCleanupCandidates}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function Metric(props: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs uppercase text-muted">{props.label}</p>
      <p className="mt-1 text-sm font-semibold text-ink">{props.value}</p>
    </div>
  );
}

function TableToolbar(props: {
  labels: AdminCopy;
  search: string;
  sort: string;
  sortOptions: Array<{ label: string; value: string }>;
  onSearchChange: (value: string) => void;
  onSortChange: (value: string) => void;
}) {
  return (
    <div className="mt-4 flex flex-col gap-3 rounded-md border border-border bg-white p-3 sm:flex-row sm:items-end sm:justify-between">
      <label className="flex min-w-0 flex-1 flex-col gap-1 text-sm font-medium">
        {props.labels.search}
        <div className="flex h-10 items-center gap-2 rounded-md border border-border bg-white px-3 focus-within:border-accent">
          <Search className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
          <input
            className="min-w-0 flex-1 text-sm font-normal outline-none"
            placeholder={props.labels.searchPlaceholder}
            type="search"
            value={props.search}
            onChange={(event) => props.onSearchChange(event.target.value)}
          />
        </div>
      </label>
      <label className="flex flex-col gap-1 text-sm font-medium sm:w-64">
        {props.labels.sortBy}
        <select
          className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
          value={props.sort}
          onChange={(event) => props.onSortChange(event.target.value)}
        >
          {props.sortOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}

function RecordingsPanel(props: {
  canManageLocalFiles: boolean;
  canScanLocalFiles: boolean;
  isLoading: boolean;
  labels: AdminCopy;
  protectPending: boolean;
  reconcileError: boolean;
  reconcilePending: boolean;
  reconcileResult?: ReconcileResult;
  recordings: RecordingItem[];
  search: string;
  sort: RecordingSortKey;
  total: number;
  visibleTotal: number;
  onReconcile: () => void;
  onSearchChange: (value: string) => void;
  onSortChange: (value: RecordingSortKey) => void;
  onToggleProtect: (recording: RecordingItem) => void;
}) {
  const [selectedRecording, setSelectedRecording] = useState<RecordingItem | null>(null);
  const visibleSizeBytes = props.recordings.reduce((total, recording) => {
    return total + (recording.files ?? []).reduce((fileTotal, file) => fileTotal + file.size_bytes, 0);
  }, 0);
  const shortSegmentCount = props.recordings.filter((recording) => {
    const file = recording.files?.[0];
    const durationMs = recording.duration_ms || file?.duration_ms || 0;
    return durationMs > 0 && durationMs < 3 * 60 * 1000;
  }).length;
  const protectedCount = props.recordings.filter((recording) => recording.local_protected).length;

  return (
    <section id="recordings" className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{props.labels.recordings}</h2>
          <p className="mt-1 text-sm text-muted">{props.labels.total(props.visibleTotal)} / {props.total}</p>
        </div>
        {props.canScanLocalFiles ? (
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.reconcilePending}
            type="button"
            onClick={props.onReconcile}
          >
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            {props.labels.scan}
          </button>
        ) : null}
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <Metric label={props.labels.visibleSize} value={formatBytes(visibleSizeBytes)} />
        <Metric label={props.labels.shortSegments} value={String(shortSegmentCount)} />
        <Metric label={props.labels.protectedRecordings} value={String(protectedCount)} />
      </div>

      {props.reconcileResult ? (
        <p className="mt-3 text-sm text-muted">
          {props.labels.scanResult(
            props.reconcileResult.imported,
            props.reconcileResult.updated,
            props.reconcileResult.skipped
          )}
        </p>
      ) : null}
      {props.reconcileError ? (
        <p className="mt-3 text-sm text-red-700">{props.labels.scanFailed}</p>
      ) : null}

      <TableToolbar
        labels={props.labels}
        search={props.search}
        sort={props.sort}
        sortOptions={[
          { value: "started_desc", label: props.labels.sortNewest },
          { value: "started_asc", label: props.labels.sortOldest },
          { value: "duration_desc", label: props.labels.sortDuration },
          { value: "size_desc", label: props.labels.sortSize }
        ]}
        onSearchChange={props.onSearchChange}
        onSortChange={(value) => props.onSortChange(value as RecordingSortKey)}
      />

      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.recording}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.startTime}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.profile}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.status}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.duration}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.size}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.path}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.actions}</th>
            </tr>
          </thead>
          <tbody>
            {props.recordings.map((recording) => {
              const file = recording.files?.[0];
              const canUseLocalFile = recording.local_storage_status !== "DELETED";
              const durationMs = recording.duration_ms || file?.duration_ms || 0;
              const isShortRecording = durationMs > 0 && durationMs < 3 * 60 * 1000;
              return (
                <tr key={recording.id} className="bg-white">
                  <td className="px-3 py-3">
                    <div className="flex items-start gap-2">
                      <FileVideo className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                      <div>
                        <button
                          className="text-left font-semibold text-ink hover:text-accent"
                          type="button"
                          onClick={() => setSelectedRecording(recording)}
                        >
                          {recording.title || file?.original_name || props.labels.untitled}
                        </button>
                        {isShortRecording ? (
                          <span className="ml-2 inline-flex rounded-md border border-amber-300 px-2 py-0.5 text-xs font-medium text-amber-800">
                            {props.labels.shortRecording}
                          </span>
                        ) : null}
                        <p className="mt-1 text-xs text-muted">
                          {props.labels.completedAt}: {formatDateTime(recording.completed_at || file?.closed_at || "", props.labels)}
                        </p>
                      </div>
                    </div>
                  </td>
                  <td className="px-3 py-3 text-xs text-muted">{formatDateTime(recording.started_at, props.labels)}</td>
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">{recording.profile_name}</p>
                    <p className="mt-1 text-xs text-muted">{recording.room_id}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>{recording.recording_status}</p>
                    <p className="mt-1 text-xs">{file?.file_status ?? props.labels.noFile}</p>
                    {recording.local_protected ? <p className="mt-1 text-xs font-medium text-accent">{props.labels.protected}</p> : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatDuration(durationMs)}
                  </td>
                  <td className="px-3 py-3 text-muted">{formatBytes(file?.size_bytes ?? 0)}</td>
                  <td className="max-w-md px-3 py-3 text-xs text-muted">
                    <span className="break-all">{file?.relative_path ?? "-"}</span>
                  </td>
                  <td className="px-3 py-3">
                    {props.canManageLocalFiles ? (
                      <div className="flex flex-col items-start gap-2">
                        <button
                          className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                          type="button"
                          onClick={() => setSelectedRecording(recording)}
                        >
                          {props.labels.details}
                        </button>
                        {canUseLocalFile ? (
                          <button
                            className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                            disabled={props.protectPending}
                            type="button"
                            onClick={() => props.onToggleProtect(recording)}
                          >
                            {recording.local_protected ? (
                              <Unlock className="h-3.5 w-3.5" aria-hidden="true" />
                            ) : (
                              <Lock className="h-3.5 w-3.5" aria-hidden="true" />
                            )}
                            {recording.local_protected ? props.labels.unprotect : props.labels.protect}
                          </button>
                        ) : null}
                        {file && file.file_status === "CLOSED" ? (
                          <a
                            className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                            href={`/api/v1/recording-files/${file.id}/download`}
                          >
                            <Download className="h-3.5 w-3.5" aria-hidden="true" />
                            {props.labels.download}
                          </a>
                        ) : null}
                      </div>
                    ) : (
                      <span className="text-xs text-muted">{props.labels.noAction}</span>
                    )}
                  </td>
                </tr>
              );
            })}
            {props.recordings.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={8}>
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
      {selectedRecording ? (
        <RecordingDetailsDialog
          canManageLocalFiles={props.canManageLocalFiles}
          labels={props.labels}
          recording={selectedRecording}
          protectPending={props.protectPending}
          onClose={() => setSelectedRecording(null)}
          onToggleProtect={props.onToggleProtect}
        />
      ) : null}
    </section>
  );
}

function RecordingDetailsDialog(props: {
  canManageLocalFiles: boolean;
  labels: AdminCopy;
  protectPending: boolean;
  recording: RecordingItem;
  onClose: () => void;
  onToggleProtect: (recording: RecordingItem) => void;
}) {
  const file = props.recording.files?.[0];
  const durationMs = props.recording.duration_ms || file?.duration_ms || 0;
  const isShortRecording = durationMs > 0 && durationMs < 3 * 60 * 1000;
  const canUseLocalFile = props.recording.local_storage_status !== "DELETED";

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/35 px-4 py-6">
      <section className="w-full max-w-2xl rounded-md border border-border bg-panel p-4 shadow-xl">
        <div className="flex items-start justify-between gap-3 border-b border-border pb-3">
          <div>
            <h2 className="text-base font-semibold">{props.labels.recordingDetails}</h2>
            <p className="mt-1 text-sm font-semibold text-ink">
              {props.recording.title || file?.original_name || props.labels.untitled}
            </p>
          </div>
          <button
            className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border text-ink hover:border-accent hover:text-accent"
            type="button"
            onClick={props.onClose}
          >
            <X className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{props.labels.close}</span>
          </button>
        </div>

        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <Metric label={props.labels.profile} value={`${props.recording.profile_name} / ${props.recording.room_id}`} />
          <Metric label={props.labels.streamer} value={props.recording.streamer_name || "-"} />
          <Metric label={props.labels.startTime} value={formatDateTime(props.recording.started_at, props.labels)} />
          <Metric label={props.labels.completedAt} value={formatDateTime(props.recording.completed_at || file?.closed_at || "", props.labels)} />
          <Metric label={props.labels.duration} value={formatDuration(durationMs)} />
          <Metric label={props.labels.fileSize} value={formatBytes(file?.size_bytes ?? 0)} />
          <Metric label={props.labels.recordingStatus} value={props.recording.recording_status} />
          <Metric label={props.labels.localStorageStatus} value={props.recording.local_storage_status} />
          <Metric label={props.labels.fileStatus} value={file?.file_status ?? props.labels.noFile} />
          <Metric label={props.labels.fileKind} value={file?.kind ?? "-"} />
        </div>

        {isShortRecording ? (
          <p className="mt-4 inline-flex rounded-md border border-amber-300 px-2 py-1 text-xs font-medium text-amber-800">
            {props.labels.shortRecording}
          </p>
        ) : null}

        <div className="mt-4 overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">{props.labels.file}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.fileKind}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.fileStatus}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.fileSize}</th>
                <th className="px-3 py-2 font-semibold">{props.labels.actions}</th>
              </tr>
            </thead>
            <tbody>
              {(props.recording.files ?? []).map((item) => (
                <tr key={item.id} className="bg-white align-top">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">{item.original_name}</p>
                    <p className="mt-1 break-all text-xs text-muted">{item.relative_path}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">{item.kind}</td>
                  <td className="px-3 py-3 text-muted">{item.file_status}</td>
                  <td className="px-3 py-3 text-muted">{formatBytes(item.size_bytes)}</td>
                  <td className="px-3 py-3">
                    {item.file_status === "CLOSED" ? (
                      <a
                        className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                        href={`/api/v1/recording-files/${item.id}/download`}
                      >
                        <Download className="h-3.5 w-3.5" aria-hidden="true" />
                        {props.labels.download}
                      </a>
                    ) : (
                      <span className="text-xs text-muted">{props.labels.noAction}</span>
                    )}
                  </td>
                </tr>
              ))}
              {(props.recording.files ?? []).length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-muted" colSpan={5}>
                    {props.labels.noFile}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
          <div className="text-sm text-muted">
            {props.recording.local_protected ? props.labels.protected : props.labels.localStorageStatus}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {props.canManageLocalFiles && canUseLocalFile ? (
              <button
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                disabled={props.protectPending}
                type="button"
                onClick={() => props.onToggleProtect(props.recording)}
              >
                {props.recording.local_protected ? (
                  <Unlock className="h-4 w-4" aria-hidden="true" />
                ) : (
                  <Lock className="h-4 w-4" aria-hidden="true" />
                )}
                {props.recording.local_protected ? props.labels.unprotect : props.labels.protect}
              </button>
            ) : null}
            {file && file.file_status === "CLOSED" ? (
              <a
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-medium text-ink hover:border-accent hover:text-accent"
                href={`/api/v1/recording-files/${file.id}/download`}
              >
                <Download className="h-4 w-4" aria-hidden="true" />
                {props.labels.download}
              </a>
            ) : null}
            <button
              className="inline-flex h-9 items-center justify-center rounded-md bg-ink px-3 text-sm font-medium text-white"
              type="button"
              onClick={props.onClose}
            >
              {props.labels.close}
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}

function JobsPanel(props: {
  cancelPending: boolean;
  isLoading: boolean;
  jobs: JobItem[];
  labels: AdminCopy;
  loadError: boolean;
  retryPending: boolean;
  search: string;
  sort: JobSortKey;
  total: number;
  visibleTotal: number;
  onCancel: (job: JobItem) => void;
  onRetry: (job: JobItem) => void;
  onSearchChange: (value: string) => void;
  onSortChange: (value: JobSortKey) => void;
}) {
  return (
    <section id="jobs" className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{props.labels.jobs}</h2>
          <p className="mt-1 text-sm text-muted">{props.labels.total(props.visibleTotal)} / {props.total}</p>
        </div>
        <RefreshCw className="h-5 w-5 text-accent" aria-hidden="true" />
      </div>

      {props.loadError ? <p className="mt-3 text-sm text-red-700">{props.labels.jobsFailed}</p> : null}

      <TableToolbar
        labels={props.labels}
        search={props.search}
        sort={props.sort}
        sortOptions={[
          { value: "updated_desc", label: props.labels.sortUpdated },
          { value: "run_after_asc", label: props.labels.sortRunAfter },
          { value: "status_asc", label: props.labels.sortStatus }
        ]}
        onSearchChange={props.onSearchChange}
        onSortChange={(value) => props.onSortChange(value as JobSortKey)}
      />

      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.job}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.profile}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.status}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.attempts}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.runAfter}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.lastError}</th>
              <th className="px-3 py-2 font-semibold">{props.labels.actions}</th>
            </tr>
          </thead>
          <tbody>
            {props.jobs.map((job) => {
              const canRetry = job.status === "FAILED" || job.status === "CANCELLED";
              const canCancel = !["SUCCEEDED", "CANCELLED", "RUNNING"].includes(job.status);
              return (
                <tr key={job.id} className="bg-white align-top">
                  <td className="px-3 py-3">
                    <p className="font-semibold text-ink">{job.type}</p>
                    <p className="mt-1 text-xs text-muted">{job.resource_class}</p>
                    {job.business_key ? <p className="mt-1 break-all text-xs text-muted">{job.business_key}</p> : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>{job.profile_name || "-"}</p>
                    {job.owner_username ? <p className="mt-1 text-xs">{job.owner_username}</p> : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>{job.status}</p>
                    <p className="mt-1 text-xs">{formatDateTime(job.updated_at, props.labels)}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {job.attempts} / {job.max_attempts}
                  </td>
                  <td className="px-3 py-3 text-xs text-muted">{formatDateTime(job.run_after, props.labels)}</td>
                  <td className="max-w-md px-3 py-3 text-xs text-muted">
                    <span className="break-words">{job.last_error || job.last_error_class || "-"}</span>
                  </td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
                      {canRetry ? (
                        <button
                          className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                          disabled={props.retryPending}
                          type="button"
                          onClick={() => props.onRetry(job)}
                        >
                          <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
                          {props.labels.retry}
                        </button>
                      ) : null}
                      {canCancel ? (
                        <button
                          className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                          disabled={props.cancelPending}
                          type="button"
                          onClick={() => props.onCancel(job)}
                        >
                          {props.labels.cancel}
                        </button>
                      ) : null}
                      {!canRetry && !canCancel ? <span className="text-xs text-muted">{props.labels.noAction}</span> : null}
                    </div>
                  </td>
                </tr>
              );
            })}
            {props.jobs.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={7}>
                  {props.isLoading
                    ? props.labels.checking
                    : props.jobs.length === 0 && props.search
                      ? props.labels.emptyFiltered
                      : props.labels.noJobs}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function TextField(props: {
  autoComplete?: string;
  label: string;
  type?: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <input
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
        autoComplete={props.autoComplete}
        type={props.type ?? "text"}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </label>
  );
}

function formatBytes(value: number): string {
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

function bytesToGB(value: number): number {
  return Math.max(0, Math.round(value / 1024 / 1024 / 1024));
}

function gbToBytes(value: number): number {
  return Math.max(0, Math.round(value * 1024 * 1024 * 1024));
}

function formatDuration(value: number): string {
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

function formatDateTime(value: string, labels: AdminCopy): string {
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
    hour12: false
  }).format(date);
  return `${formatted} ${labels.chinaTime}`;
}

function NumberField(props: { label: string; max?: number; min: number; value: number; onChange: (value: number) => void }) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <input
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
        max={props.max}
        min={props.min}
        type="number"
        value={props.value}
        onChange={(event) => props.onChange(Number(event.target.value))}
      />
    </label>
  );
}

function SelectField(props: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <select
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      >
        <option value="original">original</option>
        <option value="high">high</option>
        <option value="medium">medium</option>
      </select>
    </label>
  );
}

function ToggleField(props: { disabled?: boolean; label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 rounded-md border border-border bg-white px-3 py-2 text-sm font-medium">
      {props.label}
      <input
        className="h-4 w-4 accent-[#16867a]"
        checked={props.checked}
        disabled={props.disabled}
        type="checkbox"
        onChange={(event) => props.onChange(event.target.checked)}
      />
    </label>
  );
}
