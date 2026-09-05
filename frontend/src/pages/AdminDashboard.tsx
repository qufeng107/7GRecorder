import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity,
  Archive,
  Database,
  HardDrive,
  LogIn,
  LogOut,
  Plus,
  Save,
  ShieldCheck
} from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

type User = {
  id: number;
  username: string;
  role: "SUPER_ADMIN" | "MANAGER";
  enabled: boolean;
};

type MeResponse = {
  user: User;
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
  items: RecordingProfile[];
  total: number;
};

type ProfileForm = {
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

const emptyProfileForm: ProfileForm = {
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
  const [selectedProfileId, setSelectedProfileId] = useState<number | null>(null);
  const [profileForm, setProfileForm] = useState<ProfileForm>(emptyProfileForm);

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false
  });

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
    retry: false
  });

  useEffect(() => {
    const selected = profilesQuery.data?.items.find((profile) => profile.id === selectedProfileId);
    if (selected) {
      setProfileForm(profileToForm(selected));
    }
  }, [profilesQuery.data?.items, selectedProfileId]);

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
      setSelectedProfileId(null);
      setProfileForm(emptyProfileForm);
      queryClient.removeQueries({ queryKey: ["recording-profiles"] });
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
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
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
      setProfileForm(emptyProfileForm);
      void queryClient.invalidateQueries({ queryKey: ["recording-profiles"] });
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

  const user = meQuery.data?.user;
  const profiles = profilesQuery.data?.items ?? [];

  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <header className="flex flex-col gap-2 border-b border-border pb-5">
          <p className="text-sm font-medium text-accent">7GRecorder Admin</p>
          <h1 className="text-3xl font-semibold tracking-normal">Recorder Console</h1>
        </header>

        <section className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {statusRows.map(({ label, value, icon: Icon }) => (
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
          </div>

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
        </section>

        {user ? (
          <section className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
            <ProfileFormPanel
              form={profileForm}
              isEditing={Boolean(selectedProfileId)}
              isSaving={saveProfileMutation.isPending}
              saveError={saveProfileMutation.isError}
              onCancel={() => {
                setSelectedProfileId(null);
                setProfileForm(emptyProfileForm);
              }}
              onChange={setProfileForm}
              onSubmit={onProfileSubmit}
            />

            <ProfileListPanel
              archivePending={archiveProfileMutation.isPending}
              profiles={profiles}
              selectedProfileId={selectedProfileId}
              total={profilesQuery.data?.total ?? 0}
              onArchive={(profileId) => archiveProfileMutation.mutate(profileId)}
              onSelect={(profile) => setSelectedProfileId(profile.id)}
            />
          </section>
        ) : null}

        <section className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-border pt-4 text-sm text-muted">
          <span>API: {healthQuery.data?.status ?? "checking"}</span>
          <span>Release: {healthQuery.data?.release_sha ?? "unknown"}</span>
        </section>
      </div>
    </main>
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

function ProfileFormPanel(props: {
  form: ProfileForm;
  isEditing: boolean;
  isSaving: boolean;
  saveError: boolean;
  onCancel: () => void;
  onChange: (form: ProfileForm) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const update = <K extends keyof ProfileForm>(key: K, value: ProfileForm[K]) => {
    props.onChange({ ...props.form, [key]: value });
  };

  return (
    <form className="rounded-md border border-border bg-panel p-4 shadow-sm" onSubmit={props.onSubmit}>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">{props.isEditing ? "Edit Profile" : "New Profile"}</h2>
        <button
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
          disabled={props.isSaving}
          type="submit"
        >
          {props.isEditing ? <Save className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
          {props.isEditing ? "Save" : "Create"}
        </button>
      </div>

      <div className="mt-4 grid gap-3">
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
      {props.isEditing ? (
        <button className="mt-3 text-sm font-medium text-muted hover:text-ink" type="button" onClick={props.onCancel}>
          Clear selection
        </button>
      ) : null}
    </form>
  );
}

function ProfileListPanel(props: {
  archivePending: boolean;
  profiles: RecordingProfile[];
  selectedProfileId: number | null;
  total: number;
  onArchive: (profileId: number) => void;
  onSelect: (profile: RecordingProfile) => void;
}) {
  return (
    <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">Recording Profiles</h2>
        <span className="text-sm text-muted">{props.total} total</span>
      </div>
      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">Name</th>
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
                <td className="px-3 py-3 text-muted">{profile.room_id}</td>
                <td className="px-3 py-3 text-muted">{profile.runtime.recorder_status}</td>
                <td className="px-3 py-3 text-muted">{profile.runtime.sync_status}</td>
                <td className="px-3 py-3">
                  {profile.archived_at ? (
                    <span className="text-xs text-muted">Archived</span>
                  ) : (
                    <button
                      className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink disabled:opacity-60"
                      disabled={props.archivePending}
                      type="button"
                      onClick={() => props.onArchive(profile.id)}
                    >
                      Archive
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {props.profiles.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={5}>
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

function NumberField(props: { label: string; min: number; value: number; onChange: (value: number) => void }) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <input
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
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

function ToggleField(props: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 rounded-md border border-border bg-white px-3 py-2 text-sm font-medium">
      {props.label}
      <input
        className="h-4 w-4 accent-[#16867a]"
        checked={props.checked}
        type="checkbox"
        onChange={(event) => props.onChange(event.target.checked)}
      />
    </label>
  );
}
