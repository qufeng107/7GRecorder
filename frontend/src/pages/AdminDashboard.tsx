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
  Settings,
  ShieldCheck,
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

type AdminPage = "overview" | "profiles" | "recordings" | "system" | "accounts" | "me";

type AccountForm = {
  username: string;
  password: string;
  enabled: boolean;
  policy: ManagerPolicy;
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

const statusRows = [
  { label: "Recording Core", value: "Profiles ready", icon: Activity },
  { label: "SQLite", value: "Migrated on deploy", icon: Database },
  { label: "Local Storage", value: "Always enabled", icon: HardDrive },
  { label: "Optional Modules", value: "Disabled until configured", icon: Archive },
  { label: "Deployment", value: "main-only production", icon: ShieldCheck }
];

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

export function AdminDashboard() {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [activePage, setActivePage] = useState<AdminPage>("overview");
  const [selectedProfileId, setSelectedProfileId] = useState<number | null>(null);
  const [profileEditorOpen, setProfileEditorOpen] = useState(false);
  const [profileForm, setProfileForm] = useState<ProfileForm>(emptyProfileForm);
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
  const accounts = accountsQuery.data?.items ?? [];
  const accountTotal = accountsQuery.data?.total ?? accounts.length;

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
      queryClient.removeQueries({ queryKey: ["recording-profiles"] });
      queryClient.removeQueries({ queryKey: ["recordings"] });
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
    mutationFn: (request: { accountId: number; enabled: boolean }) =>
      requestJson<Account>(`/api/v1/accounts/${request.accountId}`, {
        method: "PATCH",
        body: JSON.stringify({ enabled: request.enabled })
      }),
    onSuccess: () => {
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

  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <header className="border-b border-border pb-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <p className="text-sm font-medium text-accent">7GRecorder Admin</p>
              <h1 className="mt-2 text-3xl font-semibold tracking-normal">Recorder Console</h1>
            </div>
            {user ? (
              <AccountMenu
                onAccount={() => setActivePage("me")}
                logoutPending={logoutMutation.isPending}
                user={user}
                onLogout={() => logoutMutation.mutate()}
              />
            ) : null}
          </div>
          {user ? (
            <AdminNav
              activePage={activePage}
              canManageSystemSettings={Boolean(canManageSystemSettings)}
              onChange={setActivePage}
            />
          ) : null}
        </header>

        {user ? null : (
          <SessionPanel
            loginError={loginMutation.isError}
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
            {activePage === "overview" ? <OverviewPanel statusRows={statusRows} /> : null}

            {activePage === "me" ? (
              <MyAccountPanel
                canManageSystemSettings={Boolean(canManageSystemSettings)}
                policy={ownPolicy}
                user={user}
              />
            ) : null}

            {activePage === "profiles" ? (
              <ProfileListPanel
                profiles={profiles}
                selectedProfileId={selectedProfileId}
                total={profileTotal}
                showOwner={Boolean(canManageSystemSettings)}
                onCreate={() => {
                  setSelectedProfileId(null);
                  setProfileForm({ ...emptyProfileForm, owner_user_id: String(user.id) });
                  setProfileEditorOpen(true);
                }}
                onSelect={(profile) => {
                  setSelectedProfileId(profile.id);
                  setProfileForm(profileToForm(profile));
                  setProfileEditorOpen(true);
                }}
              />
            ) : null}

            {activePage === "accounts" && canManageSystemSettings ? (
              <AccountsPanel
                accountForm={accountForm}
                accounts={accounts}
                createError={createAccountMutation.isError}
                createPending={createAccountMutation.isPending}
                currentUserId={user.id}
                policyPending={updatePolicyMutation.isPending}
                total={accountTotal}
                updatePending={updateAccountMutation.isPending}
                onAccountFormChange={setAccountForm}
                onCreate={onAccountSubmit}
                onToggleEnabled={(account) =>
                  updateAccountMutation.mutate({ accountId: account.id, enabled: !account.enabled })
                }
                onUpdatePolicy={(account, policy) =>
                  updatePolicyMutation.mutate({ accountId: account.id, policy })
                }
              />
            ) : null}

            {profileEditorOpen ? (
              <ProfileEditorDialog
                archivePending={archiveProfileMutation.isPending}
                form={profileForm}
                isEditing={Boolean(selectedProfileId)}
                isSaving={saveProfileMutation.isPending}
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
                isLoading={localStorageQuery.isLoading}
                isSaving={saveStorageSettingsMutation.isPending}
                saveError={saveStorageSettingsMutation.isError}
                status={localStorageQuery.data}
                onFormChange={setStorageForm}
                onSave={() => saveStorageSettingsMutation.mutate()}
              />
            ) : null}

            {activePage === "recordings" ? (
              <RecordingsPanel
                isLoading={recordingsQuery.isLoading}
                protectPending={protectRecordingMutation.isPending}
                reconcileError={reconcileMutation.isError}
                reconcilePending={reconcileMutation.isPending}
                reconcileResult={reconcileMutation.data}
                recordings={recordings}
                total={recordingTotal}
                onReconcile={() => reconcileMutation.mutate()}
                onToggleProtect={(recording) =>
                  protectRecordingMutation.mutate({ id: recording.id, protected: !recording.local_protected })
                }
              />
            ) : null}
          </>
        ) : null}

        <section className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-border pt-4 text-sm text-muted">
          <span>API: {healthQuery.data?.status ?? "checking"}</span>
          <span>Release: {healthQuery.data?.release_sha ?? "unknown"}</span>
        </section>
      </div>
    </main>
  );
}

function AdminNav(props: {
  activePage: AdminPage;
  canManageSystemSettings: boolean;
  onChange: (page: AdminPage) => void;
}) {
  const items: Array<{ page: AdminPage; label: string; icon: typeof Activity }> = [
    { page: "overview", label: "Overview", icon: LayoutDashboard },
    { page: "profiles", label: "Profiles", icon: Activity },
    { page: "recordings", label: "Recordings", icon: FileVideo }
  ];

  if (props.canManageSystemSettings) {
    items.push({ page: "accounts", label: "Accounts", icon: Users });
    items.push({ page: "system", label: "System Settings", icon: Settings });
  }

  return (
    <nav className="mt-5 flex flex-wrap gap-2" aria-label="Admin sections">
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

function AccountMenu(props: { logoutPending: boolean; user: User; onAccount: () => void; onLogout: () => void }) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-panel px-3 py-2 shadow-sm">
      <UserCircle className="h-5 w-5 text-accent" aria-hidden="true" />
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{props.user.username}</p>
        <p className="text-xs text-muted">{props.user.role}</p>
      </div>
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-medium text-ink hover:border-accent hover:text-accent"
        type="button"
        onClick={props.onAccount}
      >
        <UserCircle className="h-4 w-4" aria-hidden="true" />
        My Account
      </button>
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-ink px-3 text-sm font-medium text-white disabled:opacity-60"
        disabled={props.logoutPending}
        type="button"
        onClick={props.onLogout}
      >
        <LogOut className="h-4 w-4" aria-hidden="true" />
        Sign out
      </button>
    </div>
  );
}

function MyAccountPanel(props: {
  canManageSystemSettings: boolean;
  policy?: ManagerPolicy;
  user: User;
}) {
  return (
    <section className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">My Account</h2>
        <div className="mt-4 grid gap-3">
          <Metric label="Username" value={props.user.username} />
          <Metric label="Role" value={props.user.role} />
          <Metric label="Status" value={props.user.enabled ? "ENABLED" : "DISABLED"} />
        </div>
      </section>

      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">Access</h2>
        {props.canManageSystemSettings ? (
          <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <Metric label="System Settings" value="Allowed" />
            <Metric label="Accounts" value="Allowed" />
            <Metric label="Profiles" value="All owners" />
          </div>
        ) : props.policy ? (
          <div className="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-3">
            <PermissionBadge label="Recording Profiles" enabled={props.policy.can_edit_recording_profile} />
            <PermissionBadge label="Bilibili Config" enabled={props.policy.can_edit_bilibili_module} />
            <PermissionBadge label="COS Config" enabled={props.policy.can_edit_cos_module} />
            <PermissionBadge label="NetEase Config" enabled={props.policy.can_edit_netease_module} />
            <PermissionBadge label="Local Files" enabled={props.policy.can_manage_local_files} />
          </div>
        ) : (
          <p className="mt-4 text-sm text-muted">Access policy is not available.</p>
        )}
      </section>
    </section>
  );
}

function PermissionBadge(props: { enabled: boolean; label: string }) {
  return (
    <div className="rounded-md border border-border bg-white px-3 py-2">
      <p className="text-xs uppercase text-muted">{props.label}</p>
      <p className={`mt-1 text-sm font-semibold ${props.enabled ? "text-accent" : "text-muted"}`}>
        {props.enabled ? "Allowed" : "Blocked"}
      </p>
    </div>
  );
}

function OverviewPanel(props: { statusRows: typeof statusRows }) {
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
        <h2 className="text-sm font-semibold">Session</h2>
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
            Sign out
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <h2 className="text-sm font-semibold">Session</h2>
      <form className="mt-4 flex flex-col gap-3" onSubmit={props.onLoginSubmit}>
        <TextField
          autoComplete="username"
          label="Username"
          value={props.username}
          onChange={props.onUsernameChange}
        />
        <TextField
          autoComplete="current-password"
          label="Password"
          type="password"
          value={props.password}
          onChange={props.onPasswordChange}
        />
        {props.loginError ? (
          <p className="text-sm text-red-700">Login failed. Check the credentials.</p>
        ) : null}
        <button className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white">
          <LogIn className="h-4 w-4" aria-hidden="true" />
          Sign in
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
            <h2 className="text-base font-semibold">{props.isEditing ? "Edit Profile" : "New Profile"}</h2>
            {isArchived ? <p className="mt-1 text-xs font-medium text-muted">Archived profile</p> : null}
          </div>
          <button
            className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border text-ink hover:border-accent hover:text-accent"
            type="button"
            onClick={props.onCancel}
          >
            <X className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">Close</span>
          </button>
        </div>

        <div className="mt-4 grid gap-3">
          {props.showOwner ? (
            <label className="flex flex-col gap-1 text-sm font-medium">
              Owner
              <select
                className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
                value={props.form.owner_user_id}
                onChange={(event) => update("owner_user_id", event.target.value)}
              >
                {selectedOwnerMissing ? <option value={props.form.owner_user_id}>Current owner</option> : null}
                {ownerOptions.map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.username} ({account.role})
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          <TextField label="Name" value={props.form.name} onChange={(value) => update("name", value)} />
          <TextField
            label="Room ID"
            value={props.form.room_id}
            onChange={(value) => update("room_id", value)}
          />
          <TextField
            label="Streamer"
            value={props.form.streamer_name}
            onChange={(value) => update("streamer_name", value)}
          />
          <TextField
            label="Streamer UID"
            value={props.form.streamer_uid}
            onChange={(value) => update("streamer_uid", value)}
          />
          <TextField
            label="Timezone"
            value={props.form.timezone}
            onChange={(value) => update("timezone", value)}
          />
          <TextField
            label="Public Slug"
            value={props.form.public_slug}
            onChange={(value) => update("public_slug", value)}
          />
          <SelectField label="Quality" value={props.form.quality} onChange={(value) => update("quality", value)} />
          <NumberField
            label="Segment Seconds"
            min={60}
            value={props.form.segment_duration_sec}
            onChange={(value) => update("segment_duration_sec", value)}
          />
          <NumberField
            label="Finalize Grace Seconds"
            min={0}
            value={props.form.finalize_grace_period_sec}
            onChange={(value) => update("finalize_grace_period_sec", value)}
          />
          <ToggleField label="Enabled" checked={props.form.enabled} onChange={(value) => update("enabled", value)} />
          <ToggleField
            label="Auto Record"
            checked={props.form.auto_record}
            onChange={(value) => update("auto_record", value)}
          />
          <ToggleField
            label="Record Danmaku"
            checked={props.form.record_danmaku}
            onChange={(value) => update("record_danmaku", value)}
          />
          <ToggleField
            label="Public Page"
            checked={props.form.public_enabled}
            onChange={(value) => update("public_enabled", value)}
          />
        </div>

        {props.saveError ? (
          <p className="mt-3 text-sm text-red-700">Profile save failed. Check unique room and required fields.</p>
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
                Restore profile
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
                      Confirm archive
                    </button>
                    <button
                      className="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm font-medium text-ink"
                      type="button"
                      onClick={() => setConfirmArchive(false)}
                    >
                      Cancel
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
                    Archive profile
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
              Cancel
            </button>
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
              disabled={props.isSaving}
              type="submit"
            >
              {props.isEditing ? <Save className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
              {props.isEditing ? "Save" : "Create"}
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
  policyPending: boolean;
  total: number;
  updatePending: boolean;
  onAccountFormChange: (form: AccountForm) => void;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
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
          <h2 className="text-sm font-semibold">New Manager</h2>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.createPending}
            type="submit"
          >
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            Create
          </button>
        </div>
        <div className="mt-4 grid gap-3">
          <TextField
            autoComplete="username"
            label="Username"
            value={props.accountForm.username}
            onChange={(value) => updateForm("username", value)}
          />
          <TextField
            autoComplete="new-password"
            label="Initial Password"
            type="password"
            value={props.accountForm.password}
            onChange={(value) => updateForm("password", value)}
          />
          <ToggleField
            label="Enabled"
            checked={props.accountForm.enabled}
            onChange={(value) => updateForm("enabled", value)}
          />
          <AccountPolicyFields
            policy={props.accountForm.policy}
            onChange={(key, value) => updateFormPolicy(key, value)}
          />
          {props.createError ? (
            <p className="text-sm text-red-700">Account creation failed. Check username and password.</p>
          ) : null}
        </div>
      </form>

      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold">Accounts</h2>
          <span className="text-sm text-muted">{props.total} total</span>
        </div>
        <div className="mt-4 overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">Account</th>
                <th className="px-3 py-2 font-semibold">Role</th>
                <th className="px-3 py-2 font-semibold">Profiles</th>
                <th className="px-3 py-2 font-semibold">Status</th>
                <th className="px-3 py-2 font-semibold">Actions</th>
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
                  <td className="px-3 py-3 text-muted">{account.enabled ? "ENABLED" : "DISABLED"}</td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
                      <button
                        className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                        disabled={props.updatePending || account.id === props.currentUserId}
                        type="button"
                        onClick={() => props.onToggleEnabled(account)}
                      >
                        {account.enabled ? "Disable" : "Enable"}
                      </button>
                      {account.policy ? (
                        <div className="grid gap-2 pt-1">
                          <AccountPolicyFields
                            compact
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
                    No accounts yet.
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
  policy: ManagerPolicy;
  onChange: (key: PolicyFlag, value: boolean) => void;
}) {
  const fields: Array<{ key: PolicyFlag; label: string }> = [
    { key: "can_edit_recording_profile", label: "Edit profiles" },
    { key: "can_edit_bilibili_module", label: "Bilibili config" },
    { key: "can_edit_cos_module", label: "COS config" },
    { key: "can_edit_netease_module", label: "NetEase config" },
    { key: "can_manage_local_files", label: "Local files" }
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

function ProfileListPanel(props: {
  profiles: RecordingProfile[];
  selectedProfileId: number | null;
  showOwner: boolean;
  total: number;
  onCreate: () => void;
  onSelect: (profile: RecordingProfile) => void;
}) {
  return (
    <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">Recording Profiles</h2>
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted">{props.total} total</span>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white"
            type="button"
            onClick={props.onCreate}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            New
          </button>
        </div>
      </div>
      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">Name</th>
              {props.showOwner ? <th className="px-3 py-2 font-semibold">Owner</th> : null}
              <th className="px-3 py-2 font-semibold">Room</th>
              <th className="px-3 py-2 font-semibold">Runtime</th>
              <th className="px-3 py-2 font-semibold">Sync</th>
              <th className="px-3 py-2 font-semibold">Actions</th>
            </tr>
          </thead>
          <tbody>
            {props.profiles.map((profile) => (
              <tr
                key={profile.id}
                className={profile.id === props.selectedProfileId ? "bg-[#f4f7f1]" : "bg-white"}
              >
                <td className="px-3 py-3">
                  <button className="font-semibold text-ink hover:text-accent" type="button" onClick={() => props.onSelect(profile)}>
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
                  {profile.archived_at ? (
                    <span className="rounded-md border border-border px-2 py-1 text-xs font-medium text-muted">
                      Archived
                    </span>
                  ) : (
                    <button
                      className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                      type="button"
                      onClick={() => props.onSelect(profile)}
                    >
                      Edit
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {props.profiles.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={props.showOwner ? 6 : 5}>
                  No profiles yet.
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
  form: {
    maxRecordingGB: number;
    minFreeGB: number;
    emergencyFreeGB: number;
    cleanupTargetPercent: number;
  };
  isLoading: boolean;
  isSaving: boolean;
  previewReclaimableBytes: number;
  saveError: boolean;
  status?: LocalStorageStatus;
  onFormChange: (form: {
    maxRecordingGB: number;
    minFreeGB: number;
    emergencyFreeGB: number;
    cleanupTargetPercent: number;
  }) => void;
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
          <h2 className="text-sm font-semibold">Local Storage</h2>
          <p className="mt-1 text-sm text-muted">{props.status?.data_root ?? "Checking storage."}</p>
        </div>
        <HardDrive className="h-5 w-5 shrink-0 text-accent" aria-hidden="true" />
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <Metric label="Indexed Videos" value={props.isLoading ? "..." : String(props.status?.indexed_video_files ?? 0)} />
        <Metric label="Indexed Size" value={formatBytes(props.status?.indexed_video_bytes ?? 0)} />
        <Metric label="Disk Available" value={formatBytes(props.status?.disk_available_bytes ?? 0)} />
        <Metric label="Protected" value={String(props.status?.protected_recordings ?? 0)} />
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <Metric label="Health" value={props.status?.health ?? "CHECKING"} />
        <Metric label="Need Reclaim" value={formatBytes(props.status?.need_reclaim_bytes ?? 0)} />
        <Metric label="Preview Reclaimable" value={formatBytes(props.previewReclaimableBytes)} />
      </div>

      <div className="mt-4 h-2 overflow-hidden rounded-full bg-[#e6ebe4]">
        <div className="h-full bg-accent" style={{ width: `${Math.min(100, Math.max(0, usedPercent))}%` }} />
      </div>
      <p className="mt-2 text-xs text-muted">
        Disk used: {usedPercent}% of {formatBytes(props.status?.disk_total_bytes ?? 0)}. Completed recordings:{" "}
        {props.status?.completed_recordings ?? 0}. Settings:{" "}
        {props.status?.settings_configured ? "configured" : "derived default"}.
      </p>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">Storage Settings</h3>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.isSaving}
            type="button"
            onClick={props.onSave}
          >
            <Save className="h-4 w-4" aria-hidden="true" />
            Save
          </button>
        </div>
        <div className="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <NumberField
            label="Max Recording GB"
            min={1}
            value={props.form.maxRecordingGB}
            onChange={(value) => update("maxRecordingGB", value)}
          />
          <NumberField
            label="Min Free GB"
            min={1}
            value={props.form.minFreeGB}
            onChange={(value) => update("minFreeGB", value)}
          />
          <NumberField
            label="Emergency Free GB"
            min={1}
            value={props.form.emergencyFreeGB}
            onChange={(value) => update("emergencyFreeGB", value)}
          />
          <NumberField
            label="Cleanup Target %"
            max={99}
            min={1}
            value={props.form.cleanupTargetPercent}
            onChange={(value) => update("cleanupTargetPercent", value)}
          />
        </div>
        {props.saveError ? (
          <p className="mt-3 text-sm text-red-700">Storage settings save failed. Check the thresholds.</p>
        ) : null}
      </div>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">Cleanup Preview</h3>
          <span className="text-xs text-muted">Oldest unprotected completed recordings</span>
        </div>
        <div className="mt-3 overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">Recording</th>
                <th className="px-3 py-2 font-semibold">Closed</th>
                <th className="px-3 py-2 font-semibold">Files</th>
                <th className="px-3 py-2 font-semibold">Reclaimable</th>
              </tr>
            </thead>
            <tbody>
              {props.candidates.map((candidate) => (
                <tr key={candidate.recording_id} className="bg-white">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">{candidate.title || candidate.streamer_name || "Untitled"}</p>
                    <p className="mt-1 text-xs text-muted">
                      {candidate.profile_name} - {candidate.room_id}
                    </p>
                  </td>
                  <td className="px-3 py-3 text-xs text-muted">{formatDateTime(candidate.completed_at || "")}</td>
                  <td className="px-3 py-3 text-muted">{candidate.file_count}</td>
                  <td className="px-3 py-3 text-muted">{formatBytes(candidate.reclaimable_bytes)}</td>
                </tr>
              ))}
              {props.candidates.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-muted" colSpan={4}>
                    No cleanup candidates.
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

function RecordingsPanel(props: {
  isLoading: boolean;
  protectPending: boolean;
  reconcileError: boolean;
  reconcilePending: boolean;
  reconcileResult?: ReconcileResult;
  recordings: RecordingItem[];
  total: number;
  onReconcile: () => void;
  onToggleProtect: (recording: RecordingItem) => void;
}) {
  return (
    <section id="recordings" className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">Recordings</h2>
          <p className="mt-1 text-sm text-muted">{props.total} total</p>
        </div>
        <button
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
          disabled={props.reconcilePending}
          type="button"
          onClick={props.onReconcile}
        >
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
          Scan
        </button>
      </div>

      {props.reconcileResult ? (
        <p className="mt-3 text-sm text-muted">
          Scan: {props.reconcileResult.imported} imported, {props.reconcileResult.updated} updated,{" "}
          {props.reconcileResult.skipped} ignored.
        </p>
      ) : null}
      {props.reconcileError ? (
        <p className="mt-3 text-sm text-red-700">Scan failed. Check server logs.</p>
      ) : null}

      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">Recording</th>
              <th className="px-3 py-2 font-semibold">Profile</th>
              <th className="px-3 py-2 font-semibold">Status</th>
              <th className="px-3 py-2 font-semibold">Duration</th>
              <th className="px-3 py-2 font-semibold">Size</th>
              <th className="px-3 py-2 font-semibold">Path</th>
              <th className="px-3 py-2 font-semibold">Actions</th>
            </tr>
          </thead>
          <tbody>
            {props.recordings.map((recording) => {
              const file = recording.files?.[0];
              return (
                <tr key={recording.id} className="bg-white">
                  <td className="px-3 py-3">
                    <div className="flex items-start gap-2">
                      <FileVideo className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                      <div>
                        <p className="font-semibold text-ink">{recording.title || file?.original_name || "Untitled"}</p>
                        <p className="mt-1 text-xs text-muted">Start: {formatDateTime(recording.started_at)}</p>
                        <p className="mt-1 text-xs text-muted">
                          Closed: {formatDateTime(recording.completed_at || file?.closed_at || "")}
                        </p>
                      </div>
                    </div>
                  </td>
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">{recording.profile_name}</p>
                    <p className="mt-1 text-xs text-muted">{recording.room_id}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>{recording.recording_status}</p>
                    <p className="mt-1 text-xs">{file?.file_status ?? "NO_FILE"}</p>
                    {recording.local_protected ? <p className="mt-1 text-xs font-medium text-accent">PROTECTED</p> : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatDuration(recording.duration_ms || file?.duration_ms || 0)}
                  </td>
                  <td className="px-3 py-3 text-muted">{formatBytes(file?.size_bytes ?? 0)}</td>
                  <td className="max-w-md px-3 py-3 text-xs text-muted">
                    <span className="break-all">{file?.relative_path ?? "-"}</span>
                  </td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
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
                        {recording.local_protected ? "Unprotect" : "Protect"}
                      </button>
                      {file && file.file_status !== "WRITING" ? (
                        <a
                          className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                          href={`/api/v1/recording-files/${file.id}/download`}
                        >
                          <Download className="h-3.5 w-3.5" aria-hidden="true" />
                          Download
                        </a>
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
            {props.recordings.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={7}>
                  {props.isLoading ? "Loading recordings." : "No recordings indexed yet."}
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

function formatDateTime(value: string): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return `${new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false
  }).format(date)} 中国时间`;
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
