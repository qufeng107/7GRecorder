import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import {
  type SongSettingsForm,
  type MeResponse,
  type CredentialListResponse,
  type SongSettings,
  type SongSourceListResponse,
  type SongRunListResponse,
  type RecognizedSongListResponse,
  type Credential,
  type SongAnalysisRun,
} from "../../shared/console/types";
import { emptySongSettingsForm } from "./model";
import { requestJson } from "../../shared/api/client";
import { hasManagerPermission } from "../../shared/console/format";
import { uiCopy } from "../../shared/console/copy";
import { parseConfigJSON } from "../uploads/model";
import { SongsPanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function SongsPage() {
  const language = useLanguage();
  const activePage: string = "songs";
  const queryClient = useQueryClient();
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
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false,
  });
  const user = meQuery.data?.user;
  const ownPolicy = meQuery.data?.policy;
  const canManageSystemSettings = user?.role === "SUPER_ADMIN";
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
  const ui = uiCopy[language];
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
  const credentials = useMemo(
    () => credentialsQuery.data?.items ?? [],
    [credentialsQuery.data?.items],
  );
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
  if (!user) return null;
  if (
    credentialsQuery.isError ||
    songSettingsQuery.isError ||
    songSourcesQuery.isError ||
    songRunsQuery.isError ||
    recognizedSongsQuery.isError
  )
    return (
      <PageStatus
        retry={() => {
          void credentialsQuery.refetch();
          void songSettingsQuery.refetch();
          void songSourcesQuery.refetch();
          void songRunsQuery.refetch();
          void recognizedSongsQuery.refetch();
        }}
      />
    );
  if (
    credentialsQuery.isLoading ||
    songSettingsQuery.isLoading ||
    songSourcesQuery.isLoading ||
    songRunsQuery.isLoading ||
    recognizedSongsQuery.isLoading
  )
    return <PageStatus loading />;
  return (
    <div className="feature-page space-y-6">
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
    </div>
  );
}
