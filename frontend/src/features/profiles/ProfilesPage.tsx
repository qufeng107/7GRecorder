import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState, type FormEvent } from "react";
import {
  type ProfileSortKey,
  type ProfileForm,
  type MeResponse,
  type ProfileListResponse,
  type AccountListResponse,
  type RecordingProfile,
  type RecordingSettings,
} from "../../shared/console/types";
import { emptyProfileForm, filterProfiles, profilePayload } from "./model";
import { requestJson } from "../../shared/api/client";
import {
  hasManagerPermission,
  profileToForm,
} from "../../shared/console/format";
import { uiCopy } from "../../shared/console/copy";
import { ProfileListPanel, ProfileEditorDialog } from "./views";
import { useLanguage } from "../../app/preferences";

export default function ProfilesPage() {
  const language = useLanguage();
  const queryClient = useQueryClient();
  const [profileSearch, setProfileSearch] = useState("");
  const [profileSort, setProfileSort] = useState<ProfileSortKey>("name_asc");
  const [selectedProfileId, setSelectedProfileId] = useState<number | null>(
    null,
  );
  const [profileEditorOpen, setProfileEditorOpen] = useState(false);
  const [profileForm, setProfileForm] = useState<ProfileForm>(emptyProfileForm);
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
  const ui = uiCopy[language];
  const profilesQuery = useQuery({
    queryKey: ["recording-profiles"],
    queryFn: () =>
      requestJson<ProfileListResponse>("/api/v1/recording-profiles"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 10000,
  });
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: () => requestJson<AccountListResponse>("/api/v1/accounts"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000,
  });
  const profiles = useMemo(
    () => profilesQuery.data?.items ?? [],
    [profilesQuery.data?.items],
  );
  const profileTotal = profilesQuery.data?.total ?? profiles.length;
  const selectedProfile = profiles.find(
    (profile) => profile.id === selectedProfileId,
  );
  const accounts = useMemo(
    () => accountsQuery.data?.items ?? [],
    [accountsQuery.data?.items],
  );
  const visibleProfiles = filterProfiles(profiles, profileSearch, profileSort);
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
  const onProfileSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    saveProfileMutation.mutate();
  };
  if (!user) return null;
  if (profilesQuery.isError || accountsQuery.isError)
    return (
      <PageStatus
        retry={() => {
          void profilesQuery.refetch();
          void accountsQuery.refetch();
        }}
      />
    );
  if (profilesQuery.isLoading || accountsQuery.isLoading)
    return <PageStatus loading />;
  return (
    <div className="feature-page space-y-6">
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
    </div>
  );
}
