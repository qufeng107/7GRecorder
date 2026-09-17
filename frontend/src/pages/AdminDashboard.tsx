import {
  type AdminPage,
  type Language,
  type ProfileSortKey,
  type RecordingSortKey,
  type AccountSortKey,
  type JobSortKey,
  type ProfileForm,
  type AccountEditForm,
  type CredentialForm,
  type SiteTLSForm,
  type SongSettingsForm,
  type UploadSettingsForm,
  type AccountForm,
  type MeResponse,
  type HealthResponse,
  type ProfileListResponse,
  type UploadSourceListResponse,
  type RecordingListResponse,
  type JobListResponse,
  type AccountListResponse,
  type LocalStorageStatus,
  type CleanupCandidateListResponse,
  type CredentialListResponse,
  type SiteTLSSettings,
  type SongSettings,
  type SongSourceListResponse,
  type SongRunListResponse,
  type RecognizedSongListResponse,
  type RecordingProfile,
  type RecordingItem,
  type BilibiliPublishingConfig,
  type COSStorageConfig,
  type LocalStorageSettings,
  type RecordingSettings,
  type ReconcileResult,
  type UploadSourceDiscoverResult,
  type UploadSourceRegroupResult,
  type UploadSourceRepairResult,
  type UploadSourceItem,
  type UploadSourceEditCut,
  type COSDownloadURLResponse,
  type CleanupRunResult,
  type JobItem,
  type Credential,
  type SongAnalysisRun,
  type UploadModuleReconcileResult,
  type Account,
  type ManagerPolicy,
} from "../shared/console/types";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  emptyProfileForm,
  filterProfiles,
  profilePayload,
} from "../features/profiles/model";
import {
  emptyAccountEditForm,
  emptyAccountForm,
  defaultManagerPolicy,
  filterAccounts,
  accountUpdatePayload,
} from "../features/accounts/model";
import {
  emptyCredentialForm,
  emptyUploadSettingsForm,
  bilibiliSettingsFromConfig,
  parseConfigJSON,
  bilibiliSettingsPayload,
} from "../features/uploads/model";
import { emptySiteTLSForm } from "../features/system/model";
import { emptySongSettingsForm } from "../features/songs/model";
import { requestJson } from "../shared/api/client";
import {
  hasManagerPermission,
  UPLOAD_SOURCE_MERGE_GAP_SECONDS,
  profileToForm,
  accountToEditForm,
  bytesToGB,
  chinaDateFromTimestamp,
  gbToBytes,
} from "../shared/console/format";
import { uiCopy } from "../shared/console/copy";
import {
  uploadSourceToRecordingItem,
  recordingToActiveRecordingItem,
  filterRecordings,
  latestRecordingChinaDate,
  currentChinaDate,
  parseEditCutDraft,
} from "../features/recordings/model";
import { filterJobs } from "../features/jobs/model";
import {
  AccountMenu,
  LanguageControl,
  AdminNav,
  SessionPanel,
} from "../features/legacy/views";
import { OverviewPanel } from "../features/overview/views";
import {
  MyAccountPanel,
  AccountsPanel,
  AccountEditorDialog,
} from "../features/accounts/views";
import {
  ProfileListPanel,
  ProfileEditorDialog,
} from "../features/profiles/views";
import { SiteTLSPanel, StoragePanel } from "../features/system/views";
import { UploadSettingsPanel } from "../features/uploads/views";
import { SongsPanel } from "../features/songs/views";
import { RecordingsPanel } from "../features/recordings/views";
import { JobsPanel } from "../features/jobs/views";

export function AdminDashboard(
  props: { page?: string; onPageChange?: (page: AdminPage) => void } = {},
) {
  const queryClient = useQueryClient();
  const [language, setLanguage] = useState<Language>("zh");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [localPage, setLocalPage] = useState<AdminPage>("overview");
  const pages: AdminPage[] = [
    "overview",
    "profiles",
    "recordings",
    "uploads",
    "songs",
    "jobs",
    "system",
    "accounts",
    "me",
  ];
  const activePage: AdminPage =
    props.page && pages.includes(props.page as AdminPage)
      ? (props.page as AdminPage)
      : localPage;
  const onPageChange = props.onPageChange;
  const setActivePage = useCallback(
    (page: AdminPage) => {
      if (onPageChange) onPageChange(page);
      else setLocalPage(page);
    },
    [onPageChange],
  );
  const [profileSearch, setProfileSearch] = useState("");
  const [profileSort, setProfileSort] = useState<ProfileSortKey>("name_asc");
  const [recordingSearch, setRecordingSearch] = useState("");
  const [recordingSort, setRecordingSort] =
    useState<RecordingSortKey>("started_desc");
  const [editDrafts, setEditDrafts] = useState<Record<number, string>>({});
  const [accountSearch, setAccountSearch] = useState("");
  const [accountSort, setAccountSort] =
    useState<AccountSortKey>("username_asc");
  const [jobSearch, setJobSearch] = useState("");
  const [jobSort, setJobSort] = useState<JobSortKey>("updated_desc");
  const [selectedProfileId, setSelectedProfileId] = useState<number | null>(
    null,
  );
  const [profileEditorOpen, setProfileEditorOpen] = useState(false);
  const [profileForm, setProfileForm] = useState<ProfileForm>(emptyProfileForm);
  const [selectedAccountId, setSelectedAccountId] = useState<number | null>(
    null,
  );
  const [accountEditorOpen, setAccountEditorOpen] = useState(false);
  const [accountEditForm, setAccountEditForm] =
    useState<AccountEditForm>(emptyAccountEditForm);
  const [storageForm, setStorageForm] = useState({
    maxRecordingGB: 0,
    minFreeGB: 0,
    emergencyFreeGB: 0,
    cleanupTargetPercent: 85,
  });
  const [credentialForm, setCredentialForm] =
    useState<CredentialForm>(emptyCredentialForm);
  const [siteTLSForm, setSiteTLSForm] = useState<SiteTLSForm>(emptySiteTLSForm);
  const [tlsCredentialLabel, setTLSCredentialLabel] =
    useState("7g.chat SSL sync");
  const [tlsCredentialSecret, setTLSCredentialSecret] = useState(
    '{"secret_id":"","secret_key":""}',
  );
  const [songSettingsForm, setSongSettingsForm] = useState<SongSettingsForm>(
    emptySongSettingsForm,
  );
  const [acrCredentialLabel, setAcrCredentialLabel] = useState(
    "ACRCloud song recognition",
  );
  const [acrCredentialSecret, setAcrCredentialSecret] = useState(
    '{"access_token":""}',
  );
  const [selectedSongSourceID, setSelectedSongSourceID] = useState("");
  const [uploadSettingsForm, setUploadSettingsForm] =
    useState<UploadSettingsForm>(emptyUploadSettingsForm);
  const [accountForm, setAccountForm] = useState<AccountForm>({
    ...emptyAccountForm,
    policy: { ...defaultManagerPolicy },
  });

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false,
  });
  const user = meQuery.data?.user;
  const ownPolicy = meQuery.data?.policy;
  const canManageSystemSettings = user?.role === "SUPER_ADMIN";
  const canEditRecordingProfiles = hasManagerPermission(
    user,
    ownPolicy,
    "can_edit_recording_profile",
  );
  const canEditBilibiliModule = hasManagerPermission(
    user,
    ownPolicy,
    "can_edit_bilibili_module",
  );
  const canEditCosModule = hasManagerPermission(
    user,
    ownPolicy,
    "can_edit_cos_module",
  );
  const canManageUploadSettings = canEditBilibiliModule || canEditCosModule;
  const canManageLocalFiles = hasManagerPermission(
    user,
    ownPolicy,
    "can_manage_local_files",
  );
  const canScanLocalFiles = Boolean(canManageSystemSettings);
  const ui = uiCopy[language];

  const healthQuery = useQuery({
    queryKey: ["system-health"],
    queryFn: () => requestJson<HealthResponse>("/api/v1/system/health"),
    retry: false,
    refetchInterval: 10000,
  });

  const profilesQuery = useQuery({
    queryKey: ["recording-profiles"],
    queryFn: () =>
      requestJson<ProfileListResponse>("/api/v1/recording-profiles"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 10000,
  });

  const recordingsQuery = useQuery({
    queryKey: ["upload-sources"],
    queryFn: () =>
      requestJson<UploadSourceListResponse>(
        `/api/v1/upload-sources?merge_gap_seconds=${UPLOAD_SOURCE_MERGE_GAP_SECONDS}`,
      ),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 15000,
  });

  const rawRecordingsQuery = useQuery({
    queryKey: ["recordings"],
    queryFn: () => requestJson<RecordingListResponse>("/api/v1/recordings"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 5000,
  });

  const jobsQuery = useQuery({
    queryKey: ["jobs"],
    queryFn: () => requestJson<JobListResponse>("/api/v1/jobs?limit=100"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 5000,
  });

  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: () => requestJson<AccountListResponse>("/api/v1/accounts"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000,
  });

  const localStorageQuery = useQuery({
    queryKey: ["local-storage"],
    queryFn: () => requestJson<LocalStorageStatus>("/api/v1/storage/local"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000,
  });

  const cleanupCandidatesQuery = useQuery({
    queryKey: ["cleanup-candidates"],
    queryFn: () =>
      requestJson<CleanupCandidateListResponse>(
        "/api/v1/storage/local/cleanup-candidates?limit=5",
      ),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000,
  });

  const credentialsQuery = useQuery({
    queryKey: ["credentials"],
    queryFn: () => requestJson<CredentialListResponse>("/api/v1/credentials"),
    enabled: Boolean(
      meQuery.data?.user &&
        (canManageUploadSettings || canManageSystemSettings),
    ),
    retry: false,
    refetchInterval: 30000,
  });

  const siteTLSQuery = useQuery({
    queryKey: ["site-tls"],
    queryFn: () => requestJson<SiteTLSSettings>("/api/v1/system/site-tls"),
    enabled: Boolean(canManageSystemSettings && activePage === "system"),
    retry: false,
    refetchInterval: 10000,
  });

  const songSettingsQuery = useQuery({
    queryKey: ["song-settings"],
    queryFn: () => requestJson<SongSettings>("/api/v1/song-settings"),
    enabled: Boolean(canManageSystemSettings && activePage === "songs"),
    retry: false,
  });

  const songSourcesQuery = useQuery({
    queryKey: ["song-analysis-sources"],
    queryFn: () =>
      requestJson<SongSourceListResponse>("/api/v1/song-analysis/sources"),
    enabled: Boolean(canManageSystemSettings && activePage === "songs"),
    retry: false,
    refetchInterval: 15000,
  });

  const songRunsQuery = useQuery({
    queryKey: ["song-analysis-runs"],
    queryFn: () =>
      requestJson<SongRunListResponse>("/api/v1/song-analysis/runs"),
    enabled: Boolean(canManageSystemSettings && activePage === "songs"),
    retry: false,
    refetchInterval: 5000,
  });

  const recognizedSongsQuery = useQuery({
    queryKey: ["recognized-songs"],
    queryFn: () => requestJson<RecognizedSongListResponse>("/api/v1/songs"),
    enabled: Boolean(canManageSystemSettings && activePage === "songs"),
    retry: false,
    refetchInterval: 5000,
  });

  const profiles = useMemo(
    () => profilesQuery.data?.items ?? [],
    [profilesQuery.data?.items],
  );
  const profileTotal = profilesQuery.data?.total ?? profiles.length;
  const selectedProfile = profiles.find(
    (profile) => profile.id === selectedProfileId,
  );
  const uploadProfileId = Number(uploadSettingsForm.profile_id);
  const uploadProfile = profiles.find(
    (profile) => profile.id === uploadProfileId,
  );
  const recordings = useMemo(() => {
    const uploadSourceItems = (recordingsQuery.data?.items ?? []).map(
      uploadSourceToRecordingItem,
    );
    const representedRecordingIDs = new Set<number>();
    for (const item of uploadSourceItems) {
      for (const segment of item.source_segments ?? []) {
        representedRecordingIDs.add(segment.recording_id);
      }
    }
    const activeItems = (rawRecordingsQuery.data?.items ?? [])
      .filter(
        (recording) =>
          recording.local_storage_status !== "DELETED" &&
          !representedRecordingIDs.has(recording.id),
      )
      .map(recordingToActiveRecordingItem);
    return [...activeItems, ...uploadSourceItems].sort((left, right) => {
      return (
        new Date(right.started_at).getTime() -
        new Date(left.started_at).getTime()
      );
    });
  }, [rawRecordingsQuery.data?.items, recordingsQuery.data?.items]);
  const recordingTotal =
    (recordingsQuery.data?.total ?? 0) +
    recordings.filter((item) => item.is_active_recording).length;
  const jobs = jobsQuery.data?.items ?? [];
  const jobTotal = jobsQuery.data?.total ?? jobs.length;
  const accounts = useMemo(
    () => accountsQuery.data?.items ?? [],
    [accountsQuery.data?.items],
  );
  const accountTotal = accountsQuery.data?.total ?? accounts.length;
  const localStorageSettings = localStorageQuery.data?.settings;
  const visibleProfiles = filterProfiles(profiles, profileSearch, profileSort);
  const visibleRecordings = filterRecordings(
    recordings,
    recordingSearch,
    recordingSort,
  );
  const visibleJobs = filterJobs(jobs, jobSearch, jobSort);
  const visibleAccounts = filterAccounts(accounts, accountSearch, accountSort);
  const selectedAccount = accounts.find(
    (account) => account.id === selectedAccountId,
  );
  const credentials = useMemo(
    () => credentialsQuery.data?.items ?? [],
    [credentialsQuery.data?.items],
  );

  const bilibiliConfigQuery = useQuery({
    queryKey: ["upload-config", "bilibili", uploadProfileId],
    queryFn: () =>
      requestJson<BilibiliPublishingConfig>(
        `/api/v1/recording-profiles/${uploadProfileId}/publishing/bilibili`,
      ),
    enabled: Boolean(uploadProfileId && canEditBilibiliModule),
    retry: false,
  });

  const cosConfigQuery = useQuery({
    queryKey: ["upload-config", "cos", uploadProfileId],
    queryFn: () =>
      requestJson<COSStorageConfig>(
        `/api/v1/recording-profiles/${uploadProfileId}/storage/cos`,
      ),
    enabled: Boolean(uploadProfileId && canEditCosModule),
    retry: false,
  });

  useEffect(() => {
    if (activePage === "system" && user && !canManageSystemSettings) {
      setActivePage("overview");
    }
    if (activePage === "accounts" && user && !canManageSystemSettings) {
      setActivePage("overview");
    }
    if (activePage === "songs" && user && !canManageSystemSettings) {
      setActivePage("overview");
    }
    if (activePage === "uploads" && user && !canManageUploadSettings) {
      setActivePage("overview");
    }
  }, [
    activePage,
    canManageSystemSettings,
    canManageUploadSettings,
    user,
    setActivePage,
  ]);

  useEffect(() => {
    const settings = songSettingsQuery.data;
    if (!settings) return;
    setSongSettingsForm({
      enabled: settings.enabled,
      credential_id: settings.credential_id
        ? String(settings.credential_id)
        : "",
      region: settings.region,
      container_id: settings.container_id,
      destination_cos_storage_profile_id:
        settings.destination_cos_storage_profile_id
          ? String(settings.destination_cos_storage_profile_id)
          : "",
      songs_prefix: settings.songs_prefix,
      boundary_padding_ms: settings.boundary_padding_ms,
      algorithm_version: settings.algorithm_version,
    });
  }, [songSettingsQuery.data]);

  useEffect(() => {
    const sources = songSourcesQuery.data?.items ?? [];
    if (!selectedSongSourceID && sources.length > 0) {
      setSelectedSongSourceID(String(sources[0].cos_object_id));
      if (!songSettingsForm.destination_cos_storage_profile_id) {
        setSongSettingsForm((form) => ({
          ...form,
          destination_cos_storage_profile_id: String(
            sources[0].cos_storage_profile_id,
          ),
        }));
      }
    }
  }, [
    selectedSongSourceID,
    songSettingsForm.destination_cos_storage_profile_id,
    songSourcesQuery.data?.items,
  ]);

  useEffect(() => {
    if (uploadSettingsForm.profile_id || profiles.length === 0) {
      return;
    }
    const firstAvailableProfile =
      profiles.find((profile) => !profile.archived_at) ?? profiles[0];
    setUploadSettingsForm((form) => ({
      ...form,
      profile_id: String(firstAvailableProfile.id),
    }));
  }, [profiles, uploadSettingsForm.profile_id]);

  useEffect(() => {
    const selected = profiles.find(
      (profile) => profile.id === selectedProfileId,
    );
    if (selected) {
      setProfileForm(profileToForm(selected));
    }
  }, [profiles, selectedProfileId]);

  useEffect(() => {
    const selected = accounts.find(
      (account) => account.id === selectedAccountId,
    );
    if (selected) {
      setAccountEditForm(accountToEditForm(selected));
    }
  }, [accounts, selectedAccountId]);

  useEffect(() => {
    const settings = localStorageSettings;
    if (!settings) {
      return;
    }
    setStorageForm({
      maxRecordingGB: bytesToGB(settings.max_recording_bytes),
      minFreeGB: bytesToGB(settings.min_system_free_bytes),
      emergencyFreeGB: bytesToGB(settings.absolute_emergency_free_bytes),
      cleanupTargetPercent: Math.round(settings.cleanup_target_ratio * 100),
    });
  }, [localStorageSettings]);

  useEffect(() => {
    const settings = siteTLSQuery.data;
    if (!settings) {
      return;
    }
    setSiteTLSForm({
      enabled: settings.enabled,
      credential_id: settings.credential_id
        ? String(settings.credential_id)
        : "",
      primary_domain: settings.primary_domain,
      additional_domains: (settings.additional_domains ?? []).join("\n"),
    });
  }, [siteTLSQuery.data]);

  useEffect(() => {
    const config = bilibiliConfigQuery.data;
    if (!config) {
      return;
    }
    const settings = bilibiliSettingsFromConfig(config.settings ?? {});
    setUploadSettingsForm((form) => ({
      ...form,
      bilibili_enabled: config.enabled,
      bilibili_credential_id: config.credential_id
        ? String(config.credential_id)
        : "",
      bilibili_title_template: settings.title_template,
      bilibili_description_template: settings.description_template,
      bilibili_tags: settings.tags,
      bilibili_copyright: settings.copyright,
      bilibili_source: settings.source,
      bilibili_upload_limit: settings.upload_limit,
    }));
  }, [bilibiliConfigQuery.data]);

  useEffect(() => {
    const config = cosConfigQuery.data;
    if (!config) {
      return;
    }
    setUploadSettingsForm((form) => ({
      ...form,
      cos_enabled: config.enabled,
      cos_credential_id: config.credential_id
        ? String(config.credential_id)
        : "",
      cos_region: config.region ?? "",
      cos_bucket: config.bucket ?? "",
      cos_prefix: config.prefix ?? "",
      cos_max_managed_gb:
        bytesToGB(config.max_managed_bytes) || form.cos_max_managed_gb,
    }));
  }, [cosConfigQuery.data]);

  const loginMutation = useMutation({
    mutationFn: () =>
      requestJson<MeResponse>("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      }),
    onSuccess: () => {
      setPassword("");
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });

  const logoutMutation = useMutation({
    mutationFn: () =>
      requestJson<{ status: string }>("/api/v1/auth/logout", {
        method: "POST",
      }),
    onSuccess: () => {
      setActivePage("overview");
      setSelectedProfileId(null);
      setProfileEditorOpen(false);
      setProfileForm(emptyProfileForm);
      setSelectedAccountId(null);
      setAccountEditorOpen(false);
      setAccountEditForm(emptyAccountEditForm);
      queryClient.removeQueries({ queryKey: ["recording-profiles"] });
      queryClient.removeQueries({ queryKey: ["upload-sources"] });
      queryClient.removeQueries({ queryKey: ["jobs"] });
      queryClient.removeQueries({ queryKey: ["accounts"] });
      queryClient.removeQueries({ queryKey: ["local-storage"] });
      queryClient.removeQueries({ queryKey: ["cleanup-candidates"] });
      queryClient.removeQueries({ queryKey: ["credentials"] });
      queryClient.removeQueries({ queryKey: ["upload-config"] });
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });

  const saveProfileMutation = useMutation({
    mutationFn: async () => {
      if (selectedProfileId) {
        const updated = await requestJson<RecordingProfile>(
          `/api/v1/recording-profiles/${selectedProfileId}`,
          {
            method: "PATCH",
            body: JSON.stringify(profilePayload(profileForm)),
          },
        );
        await requestJson<RecordingSettings>(
          `/api/v1/recording-profiles/${selectedProfileId}/recording-settings`,
          {
            method: "PUT",
            body: JSON.stringify(
              profilePayload(profileForm).recording_settings,
            ),
          },
        );
        return updated;
      }
      return requestJson<RecordingProfile>("/api/v1/recording-profiles", {
        method: "POST",
        body: JSON.stringify(profilePayload(profileForm)),
      });
    },
    onSuccess: (profile) => {
      setSelectedProfileId(profile.id);
      setProfileEditorOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
    },
  });

  const reconcileMutation = useMutation({
    mutationFn: async () => {
      const reconcile = await requestJson<ReconcileResult>(
        "/api/v1/recording-files/reconcile",
        {
          method: "POST",
          body: "{}",
        },
      );
      const discover = await requestJson<UploadSourceDiscoverResult>(
        `/api/v1/upload-sources/actions/discover?merge_gap_seconds=${UPLOAD_SOURCE_MERGE_GAP_SECONDS}`,
        {
          method: "POST",
          body: "{}",
        },
      );
      return { reconcile, discover };
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    },
  });

  const regroupTodayMutation = useMutation({
    mutationFn: async () => {
      const chinaDate =
        latestRecordingChinaDate(recordings) ?? currentChinaDate();
      const profileIds = Array.from(
        new Set(
          recordings
            .filter(
              (recording) =>
                chinaDateFromTimestamp(recording.started_at) === chinaDate,
            )
            .map((recording) => recording.recording_profile_id),
        ),
      );
      const items: UploadSourceRegroupResult[] = [];
      for (const profileId of profileIds) {
        const result = await requestJson<UploadSourceRegroupResult>(
          "/api/v1/upload-sources/actions/regroup",
          {
            method: "POST",
            body: JSON.stringify({
              recording_profile_id: profileId,
              china_date: chinaDate,
              merge_gap_seconds: UPLOAD_SOURCE_MERGE_GAP_SECONDS,
            }),
          },
        );
        items.push(result);
      }
      return { china_date: chinaDate, items };
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
    },
  });

  const repairUploadSourcesMutation = useMutation({
    mutationFn: () =>
      requestJson<UploadSourceRepairResult>(
        "/api/v1/upload-sources/actions/repair",
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
    },
  });

  const protectRecordingMutation = useMutation({
    mutationFn: (request: { id: number; protected: boolean }) =>
      requestJson<RecordingItem>(
        `/api/v1/recordings/${request.id}/actions/${request.protected ? "protect-local" : "unprotect-local"}`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    },
  });

  const requireRecordingReviewMutation = useMutation({
    mutationFn: (recordingId: number) =>
      requestJson<RecordingItem>(
        `/api/v1/recordings/${recordingId}/actions/require-upload-review`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["recordings"] });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const requireUploadSourceReviewMutation = useMutation({
    mutationFn: (uploadSourceId: number) =>
      requestJson<UploadSourceItem>(
        `/api/v1/upload-sources/${uploadSourceId}/actions/require-review`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const approveUploadSourceReviewMutation = useMutation({
    mutationFn: (uploadSourceId: number) =>
      requestJson<UploadSourceItem>(
        `/api/v1/upload-sources/${uploadSourceId}/actions/approve-review`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const applyUploadSourceEditMutation = useMutation({
    mutationFn: (request: {
      uploadSourceId: number;
      cuts: UploadSourceEditCut[];
    }) =>
      requestJson<UploadSourceItem>(
        `/api/v1/upload-sources/${request.uploadSourceId}/actions/apply-edit`,
        {
          method: "POST",
          body: JSON.stringify({ cuts: request.cuts }),
        },
      ),
    onSuccess: (_result, request) => {
      setEditDrafts((current) => {
        const next = { ...current };
        delete next[request.uploadSourceId];
        return next;
      });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const cosDownloadUrlMutation = useMutation({
    mutationFn: (request: { uploadSourceId: number; outputId: number }) =>
      requestJson<COSDownloadURLResponse>(
        `/api/v1/upload-sources/${request.uploadSourceId}/outputs/${request.outputId}/actions/download-url`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: (result) => {
      window.location.assign(result.url);
    },
  });

  const cosFileDownloadUrlMutation = useMutation({
    mutationFn: (request: { fileId: number }) =>
      requestJson<COSDownloadURLResponse>(
        `/api/v1/recording-files/${request.fileId}/actions/cos-download-url`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: (result) => {
      window.location.assign(result.url);
    },
  });

  const downloadLocalUploadSourceOutput = (
    uploadSourceId: number,
    outputId: number,
  ) => {
    window.location.assign(
      `/api/v1/upload-sources/${uploadSourceId}/outputs/${outputId}/download`,
    );
  };

  const saveStorageSettingsMutation = useMutation({
    mutationFn: () =>
      requestJson<LocalStorageSettings>("/api/v1/storage/local/settings", {
        method: "PUT",
        body: JSON.stringify({
          max_recording_bytes: gbToBytes(storageForm.maxRecordingGB),
          min_system_free_bytes: gbToBytes(storageForm.minFreeGB),
          cleanup_target_ratio: storageForm.cleanupTargetPercent / 100,
          absolute_emergency_free_bytes: gbToBytes(storageForm.emergencyFreeGB),
        }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    },
  });

  const cleanupMutation = useMutation({
    mutationFn: () =>
      requestJson<CleanupRunResult>("/api/v1/storage/local/actions/cleanup", {
        method: "POST",
        body: JSON.stringify({
          max_recordings: Math.max(
            1,
            Math.min(5, cleanupCandidatesQuery.data?.items?.length ?? 5),
          ),
        }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["recordings"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    },
  });

  const retryJobMutation = useMutation({
    mutationFn: (request: {
      jobId: number;
      confirmAmbiguousBilibili: boolean;
    }) =>
      requestJson<JobItem>(`/api/v1/jobs/${request.jobId}/actions/retry`, {
        method: "POST",
        body: JSON.stringify({
          confirm_ambiguous_bilibili: request.confirmAmbiguousBilibili,
        }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const cancelJobMutation = useMutation({
    mutationFn: (jobId: number) =>
      requestJson<JobItem>(`/api/v1/jobs/${jobId}/actions/cancel`, {
        method: "POST",
        body: "{}",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const createCredentialMutation = useMutation({
    mutationFn: () => {
      const secret = parseConfigJSON(credentialForm.secret);
      return requestJson<Credential>("/api/v1/credentials", {
        method: "POST",
        body: JSON.stringify({
          scope: "USER",
          platform: credentialForm.platform,
          purpose:
            credentialForm.platform === "bilibili" ? "PUBLISHER" : "STORAGE",
          account_label: credentialForm.account_label,
          external_uid: credentialForm.external_uid,
          secret,
        }),
      });
    },
    onSuccess: () => {
      setCredentialForm(emptyCredentialForm);
      void queryClient.invalidateQueries({ queryKey: ["credentials"] });
    },
  });

  const createTLSCredentialMutation = useMutation({
    mutationFn: () =>
      requestJson<Credential>("/api/v1/credentials", {
        method: "POST",
        body: JSON.stringify({
          scope: "SYSTEM",
          platform: "tencent_ssl",
          purpose: "TLS",
          account_label: tlsCredentialLabel,
          secret: parseConfigJSON(tlsCredentialSecret),
        }),
      }),
    onSuccess: (credential) => {
      setSiteTLSForm((form) => ({
        ...form,
        credential_id: String(credential.id),
      }));
      setTLSCredentialSecret('{"secret_id":"","secret_key":""}');
      void queryClient.invalidateQueries({ queryKey: ["credentials"] });
    },
  });

  const createAcrCredentialMutation = useMutation({
    mutationFn: () =>
      requestJson<Credential>("/api/v1/credentials", {
        method: "POST",
        body: JSON.stringify({
          scope: "SYSTEM",
          platform: "acrcloud",
          purpose: "SONG_RECOGNITION",
          account_label: acrCredentialLabel,
          secret: parseConfigJSON(acrCredentialSecret),
        }),
      }),
    onSuccess: (credential) => {
      setSongSettingsForm((form) => ({
        ...form,
        credential_id: String(credential.id),
      }));
      setAcrCredentialSecret('{"access_token":""}');
      void queryClient.invalidateQueries({ queryKey: ["credentials"] });
    },
  });

  const saveSongSettingsMutation = useMutation({
    mutationFn: () =>
      requestJson<SongSettings>("/api/v1/song-settings", {
        method: "PUT",
        body: JSON.stringify({
          enabled: songSettingsForm.enabled,
          credential_id: Number(songSettingsForm.credential_id || 0),
          region: songSettingsForm.region,
          container_id: songSettingsForm.container_id,
          destination_cos_storage_profile_id: Number(
            songSettingsForm.destination_cos_storage_profile_id || 0,
          ),
          songs_prefix: songSettingsForm.songs_prefix,
          boundary_padding_ms: songSettingsForm.boundary_padding_ms,
          algorithm_version: songSettingsForm.algorithm_version,
        }),
      }),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["song-settings"] }),
  });

  const createSongRunMutation = useMutation({
    mutationFn: () =>
      requestJson<SongAnalysisRun>("/api/v1/song-analysis/runs", {
        method: "POST",
        body: JSON.stringify({ cos_object_id: Number(selectedSongSourceID) }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["song-analysis-runs"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const saveSiteTLSMutation = useMutation({
    mutationFn: () =>
      requestJson<SiteTLSSettings>("/api/v1/system/site-tls", {
        method: "PUT",
        body: JSON.stringify({
          enabled: siteTLSForm.enabled,
          credential_id: Number(siteTLSForm.credential_id || 0),
          primary_domain: siteTLSForm.primary_domain,
          additional_domains: siteTLSForm.additional_domains
            .split(/\r?\n/)
            .map((value) => value.trim())
            .filter(Boolean),
        }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["site-tls"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const syncSiteTLSMutation = useMutation({
    mutationFn: () =>
      requestJson<SiteTLSSettings>("/api/v1/system/site-tls/actions/sync", {
        method: "POST",
        body: "{}",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["site-tls"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const saveBilibiliConfigMutation = useMutation({
    mutationFn: () =>
      requestJson<BilibiliPublishingConfig>(
        `/api/v1/recording-profiles/${uploadProfileId}/publishing/bilibili`,
        {
          method: "PUT",
          body: JSON.stringify({
            enabled: uploadSettingsForm.bilibili_enabled,
            credential_id: Number(
              uploadSettingsForm.bilibili_credential_id || 0,
            ),
            settings: bilibiliSettingsPayload(uploadSettingsForm),
          }),
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["upload-config", "bilibili", uploadProfileId],
      });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const saveCosConfigMutation = useMutation({
    mutationFn: () =>
      requestJson<COSStorageConfig>(
        `/api/v1/recording-profiles/${uploadProfileId}/storage/cos`,
        {
          method: "PUT",
          body: JSON.stringify({
            enabled: uploadSettingsForm.cos_enabled,
            credential_id: Number(uploadSettingsForm.cos_credential_id || 0),
            region: uploadSettingsForm.cos_region,
            bucket: uploadSettingsForm.cos_bucket,
            prefix: uploadSettingsForm.cos_prefix,
            max_managed_bytes: gbToBytes(uploadSettingsForm.cos_max_managed_gb),
          }),
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["upload-config", "cos", uploadProfileId],
      });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });

  const reconcileUploadModulesMutation = useMutation({
    mutationFn: () =>
      requestJson<UploadModuleReconcileResult>(
        "/api/v1/upload-modules/actions/reconcile",
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
    },
  });

  const archiveProfileMutation = useMutation({
    mutationFn: (profileId: number) =>
      requestJson<RecordingProfile>(`/api/v1/recording-profiles/${profileId}`, {
        method: "PATCH",
        body: JSON.stringify({ archived: true }),
      }),
    onSuccess: () => {
      setSelectedProfileId(null);
      setProfileEditorOpen(false);
      setProfileForm(emptyProfileForm);
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
    },
  });

  const restoreProfileMutation = useMutation({
    mutationFn: (profileId: number) =>
      requestJson<RecordingProfile>(`/api/v1/recording-profiles/${profileId}`, {
        method: "PATCH",
        body: JSON.stringify({ archived: false, enabled: true }),
      }),
    onSuccess: (profile) => {
      setSelectedProfileId(profile.id);
      setProfileEditorOpen(false);
      setProfileForm(profileToForm(profile));
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
    },
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
          policy: accountForm.policy,
        }),
      }),
    onSuccess: () => {
      setAccountForm({
        ...emptyAccountForm,
        policy: { ...defaultManagerPolicy },
      });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    },
  });

  const updateAccountMutation = useMutation({
    mutationFn: (request: {
      accountId: number;
      payload: ReturnType<typeof accountUpdatePayload> | { enabled: boolean };
    }) =>
      requestJson<Account>(`/api/v1/accounts/${request.accountId}`, {
        method: "PATCH",
        body: JSON.stringify(request.payload),
      }),
    onSuccess: () => {
      setAccountEditorOpen(false);
      setSelectedAccountId(null);
      setAccountEditForm(emptyAccountEditForm);
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    },
  });

  const updatePolicyMutation = useMutation({
    mutationFn: (request: { accountId: number; policy: ManagerPolicy }) =>
      requestJson<ManagerPolicy>(
        `/api/v1/accounts/${request.accountId}/policy`,
        {
          method: "PUT",
          body: JSON.stringify(request.policy),
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    },
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
      payload: accountUpdatePayload(accountEditForm),
    });
  };

  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <header className="border-b border-border pb-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <p className="text-sm font-medium text-accent">{ui.appName}</p>
              <h1 className="mt-2 text-3xl font-semibold tracking-normal">
                {ui.title}
              </h1>
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
              <LanguageControl
                language={language}
                labels={ui}
                onLanguageChange={setLanguage}
              />
            )}
          </div>
          {user ? (
            <AdminNav
              activePage={activePage}
              canManageSystemSettings={Boolean(canManageSystemSettings)}
              canManageUploadSettings={canManageUploadSettings}
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
            {activePage === "overview" ? (
              <OverviewPanel statusRows={ui.statusRows} />
            ) : null}

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
                  setProfileForm({
                    ...emptyProfileForm,
                    owner_user_id: String(user.id),
                  });
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
                  updateAccountMutation.mutate({
                    accountId: account.id,
                    payload: { enabled: !account.enabled },
                  })
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
                onArchive={(profileId) =>
                  archiveProfileMutation.mutate(profileId)
                }
                onCancel={() => {
                  setProfileEditorOpen(false);
                  setSelectedProfileId(null);
                  setProfileForm(emptyProfileForm);
                }}
                onChange={setProfileForm}
                onRestore={(profileId) =>
                  restoreProfileMutation.mutate(profileId)
                }
                onSubmit={onProfileSubmit}
              />
            ) : null}

            {activePage === "system" && canManageSystemSettings ? (
              <div className="grid gap-4">
                <SiteTLSPanel
                  credentials={credentials.filter(
                    (item) =>
                      item.scope === "SYSTEM" &&
                      item.platform === "tencent_ssl" &&
                      item.purpose === "TLS",
                  )}
                  credentialCreateError={createTLSCredentialMutation.isError}
                  credentialCreatePending={
                    createTLSCredentialMutation.isPending
                  }
                  credentialLabel={tlsCredentialLabel}
                  credentialSecret={tlsCredentialSecret}
                  form={siteTLSForm}
                  labels={ui}
                  saveError={saveSiteTLSMutation.isError}
                  savePending={saveSiteTLSMutation.isPending}
                  settings={siteTLSQuery.data}
                  syncPending={syncSiteTLSMutation.isPending}
                  onCreateCredential={(event) => {
                    event.preventDefault();
                    createTLSCredentialMutation.mutate();
                  }}
                  onCredentialLabelChange={setTLSCredentialLabel}
                  onCredentialSecretChange={setTLSCredentialSecret}
                  onFormChange={setSiteTLSForm}
                  onSave={() => saveSiteTLSMutation.mutate()}
                  onSync={() => syncSiteTLSMutation.mutate()}
                />
                <StoragePanel
                  candidates={cleanupCandidatesQuery.data?.items ?? []}
                  previewReclaimableBytes={
                    cleanupCandidatesQuery.data?.preview_reclaimable_bytes ?? 0
                  }
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
              </div>
            ) : null}

            {activePage === "uploads" ? (
              <UploadSettingsPanel
                bilibiliConfigError={saveBilibiliConfigMutation.isError}
                bilibiliConfigPending={saveBilibiliConfigMutation.isPending}
                canEditBilibiliModule={canEditBilibiliModule}
                canEditCosModule={canEditCosModule}
                canReconcileUploadJobs={Boolean(canManageSystemSettings)}
                cosConfigError={saveCosConfigMutation.isError}
                cosConfigPending={saveCosConfigMutation.isPending}
                credentialCreateError={createCredentialMutation.isError}
                credentialCreatePending={createCredentialMutation.isPending}
                credentialForm={credentialForm}
                credentials={credentials}
                labels={ui}
                profiles={profiles}
                reconcilePending={reconcileUploadModulesMutation.isPending}
                reconcileError={reconcileUploadModulesMutation.isError}
                reconcileResult={reconcileUploadModulesMutation.data}
                selectedProfile={uploadProfile}
                settingsForm={uploadSettingsForm}
                onCredentialFormChange={setCredentialForm}
                onCreateCredential={(event) => {
                  event.preventDefault();
                  createCredentialMutation.mutate();
                }}
                onReconcile={() => reconcileUploadModulesMutation.mutate()}
                onSaveBilibiliConfig={() => saveBilibiliConfigMutation.mutate()}
                onSaveCosConfig={() => saveCosConfigMutation.mutate()}
                onSettingsFormChange={setUploadSettingsForm}
              />
            ) : null}

            {activePage === "songs" && canManageSystemSettings ? (
              <SongsPanel
                acrCredentials={credentials.filter(
                  (item) =>
                    item.scope === "SYSTEM" &&
                    item.platform === "acrcloud" &&
                    item.purpose === "SONG_RECOGNITION",
                )}
                credentialCreateError={createAcrCredentialMutation.isError}
                credentialCreatePending={createAcrCredentialMutation.isPending}
                credentialLabel={acrCredentialLabel}
                credentialSecret={acrCredentialSecret}
                form={songSettingsForm}
                labels={ui}
                runs={songRunsQuery.data?.items ?? []}
                songs={recognizedSongsQuery.data?.items ?? []}
                saveError={saveSongSettingsMutation.isError}
                savePending={saveSongSettingsMutation.isPending}
                selectedSourceID={selectedSongSourceID}
                sources={songSourcesQuery.data?.items ?? []}
                startError={createSongRunMutation.isError}
                startPending={createSongRunMutation.isPending}
                onCreateCredential={(event) => {
                  event.preventDefault();
                  createAcrCredentialMutation.mutate();
                }}
                onCredentialLabelChange={setAcrCredentialLabel}
                onCredentialSecretChange={setAcrCredentialSecret}
                onFormChange={setSongSettingsForm}
                onSave={() => saveSongSettingsMutation.mutate()}
                onSelectedSourceChange={setSelectedSongSourceID}
                onStart={() => createSongRunMutation.mutate()}
              />
            ) : null}

            {activePage === "recordings" ? (
              <RecordingsPanel
                isLoading={
                  recordingsQuery.isLoading || rawRecordingsQuery.isLoading
                }
                jobs={jobs}
                labels={ui}
                canManageLocalFiles={canManageLocalFiles}
                canScanLocalFiles={canScanLocalFiles}
                cosDownloadPending={cosDownloadUrlMutation.isPending}
                cosDownloadPendingOutputId={
                  cosDownloadUrlMutation.variables?.outputId ?? null
                }
                cosFileDownloadPending={cosFileDownloadUrlMutation.isPending}
                cosFileDownloadPendingFileId={
                  cosFileDownloadUrlMutation.variables?.fileId ?? null
                }
                protectPending={protectRecordingMutation.isPending}
                reviewPending={
                  requireRecordingReviewMutation.isPending ||
                  requireUploadSourceReviewMutation.isPending ||
                  approveUploadSourceReviewMutation.isPending ||
                  applyUploadSourceEditMutation.isPending
                }
                regroupError={regroupTodayMutation.isError}
                regroupPending={regroupTodayMutation.isPending}
                regroupResult={regroupTodayMutation.data}
                repairError={repairUploadSourcesMutation.isError}
                repairPending={repairUploadSourcesMutation.isPending}
                repairResult={repairUploadSourcesMutation.data}
                reconcileError={reconcileMutation.isError}
                reconcilePending={reconcileMutation.isPending}
                reconcileResult={reconcileMutation.data}
                recordings={visibleRecordings}
                search={recordingSearch}
                sort={recordingSort}
                total={recordingTotal}
                visibleTotal={visibleRecordings.length}
                onReconcile={() => reconcileMutation.mutate()}
                onRegroupToday={() => {
                  if (window.confirm(ui.regroupTodayConfirm)) {
                    regroupTodayMutation.mutate();
                  }
                }}
                onRepairUploadSources={() => {
                  if (window.confirm(ui.repairUploadSourcesConfirm)) {
                    repairUploadSourcesMutation.mutate();
                  }
                }}
                onDownloadOutput={(uploadSourceId, outputId) =>
                  cosDownloadUrlMutation.mutate({ uploadSourceId, outputId })
                }
                onDownloadLocalOutput={downloadLocalUploadSourceOutput}
                onDownloadFile={(fileId) =>
                  cosFileDownloadUrlMutation.mutate({ fileId })
                }
                onApproveReview={(recording) => {
                  if (recording.upload_source_id) {
                    approveUploadSourceReviewMutation.mutate(
                      recording.upload_source_id,
                    );
                  }
                }}
                editDrafts={editDrafts}
                editError={applyUploadSourceEditMutation.isError}
                editPending={applyUploadSourceEditMutation.isPending}
                onApplyEdit={(uploadSourceId, draft) => {
                  const cuts = parseEditCutDraft(draft);
                  if (cuts.length === 0) {
                    return;
                  }
                  applyUploadSourceEditMutation.mutate({
                    uploadSourceId,
                    cuts,
                  });
                }}
                onEditDraftChange={(uploadSourceId, value) =>
                  setEditDrafts((current) => ({
                    ...current,
                    [uploadSourceId]: value,
                  }))
                }
                onRequireReview={(recording) => {
                  if (recording.upload_source_id) {
                    requireUploadSourceReviewMutation.mutate(
                      recording.upload_source_id,
                    );
                    return;
                  }
                  requireRecordingReviewMutation.mutate(recording.id);
                }}
                onSearchChange={setRecordingSearch}
                onSortChange={setRecordingSort}
                onToggleProtect={(recording) =>
                  protectRecordingMutation.mutate({
                    id: recording.id,
                    protected: !recording.local_protected,
                  })
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
                onRefresh={() => void jobsQuery.refetch()}
                onRetry={(job) => {
                  const ambiguousBilibili =
                    job.type === "UPLOAD_BILIBILI" &&
                    job.last_error_class === "AMBIGUOUS";
                  if (
                    ambiguousBilibili &&
                    !window.confirm(ui.confirmAmbiguousBilibiliRetry)
                  ) {
                    return;
                  }
                  retryJobMutation.mutate({
                    jobId: job.id,
                    confirmAmbiguousBilibili: ambiguousBilibili,
                  });
                }}
                onSearchChange={setJobSearch}
                onSortChange={setJobSort}
              />
            ) : null}
          </>
        ) : null}

        <section className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-border pt-4 text-sm text-muted">
          <span>
            {ui.api}: {healthQuery.data?.status ?? ui.checking}
          </span>
          <span>
            {ui.release}: {healthQuery.data?.release_sha ?? ui.unknown}
          </span>
        </section>
      </div>
    </main>
  );
}
