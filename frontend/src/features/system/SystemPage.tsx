import { useServerDraft } from "../../shared/forms/useServerDraft";
import { UnsavedChangesGuard } from "../../shared/forms/UnsavedChangesGuard";
import { DraftFeedback } from "../../shared/forms/DraftFeedback";
import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import {
  type SiteTLSForm,
  type MeResponse,
  type LocalStorageStatus,
  type CleanupCandidateListResponse,
  type CredentialListResponse,
  type SiteTLSSettings,
  type LocalStorageSettings,
  type CleanupRunResult,
  type Credential,
} from "../../shared/console/types";
import { emptySiteTLSForm } from "./model";
import { requestJson } from "../../shared/api/client";
import {
  hasManagerPermission,
  bytesToGB,
  gbToBytes,
} from "../../shared/console/format";
import { uiCopy } from "../../shared/console/copy";
import { parseConfigJSON } from "../uploads/model";
import { SiteTLSPanel, StoragePanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function SystemPage() {
  const language = useLanguage();
  const activePage: string = "system";
  const queryClient = useQueryClient();
  const [tlsCredentialLabel, setTLSCredentialLabel] =
    useState("7g.chat SSL sync");
  const [tlsCredentialSecret, setTLSCredentialSecret] = useState(
    '{"secret_id":"","secret_key":""}',
  );
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
  const localStorageSettings = localStorageQuery.data?.settings;
  const credentials = useMemo(
    () => credentialsQuery.data?.items ?? [],
    [credentialsQuery.data?.items],
  );
  const storageDraft = useServerDraft({
    maxRecordingGB: bytesToGB(localStorageSettings?.max_recording_bytes ?? 0),
    minFreeGB: bytesToGB(localStorageSettings?.min_system_free_bytes ?? 0),
    emergencyFreeGB: bytesToGB(
      localStorageSettings?.absolute_emergency_free_bytes ?? 0,
    ),
    cleanupTargetPercent: Math.round(
      (localStorageSettings?.cleanup_target_ratio ?? 0.85) * 100,
    ),
  });
  const settings = siteTLSQuery.data;
  const tlsDraft = useServerDraft<SiteTLSForm>(
    settings
      ? {
          enabled: settings.enabled,
          credential_id: settings.credential_id
            ? String(settings.credential_id)
            : "",
          primary_domain: settings.primary_domain,
          additional_domains: (settings.additional_domains ?? []).join("\n"),
        }
      : emptySiteTLSForm,
  );
  const storageForm = storageDraft.form;
  const siteTLSForm = tlsDraft.form;
  const setStorageForm = storageDraft.change;
  const setSiteTLSForm = tlsDraft.change;
  const saveStorageSettingsMutation = useMutation({
    mutationFn: (submitted: typeof storageForm) =>
      requestJson<LocalStorageSettings>("/api/v1/storage/local/settings", {
        method: "PUT",
        body: JSON.stringify({
          max_recording_bytes: gbToBytes(submitted.maxRecordingGB),
          min_system_free_bytes: gbToBytes(submitted.minFreeGB),
          cleanup_target_ratio: submitted.cleanupTargetPercent / 100,
          absolute_emergency_free_bytes: gbToBytes(submitted.emergencyFreeGB),
        }),
      }),
    onSuccess: (saved, submitted) => {
      queryClient.setQueryData<LocalStorageStatus>(
        ["local-storage"],
        (current) => (current ? { ...current, settings: saved } : current),
      );
      storageDraft.saved(submitted);
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
  const saveSiteTLSMutation = useMutation({
    mutationFn: (submitted: SiteTLSForm) =>
      requestJson<SiteTLSSettings>("/api/v1/system/site-tls", {
        method: "PUT",
        body: JSON.stringify({
          enabled: submitted.enabled,
          credential_id: Number(submitted.credential_id || 0),
          primary_domain: submitted.primary_domain,
          additional_domains: submitted.additional_domains
            .split(/\r?\n/)
            .map((value) => value.trim())
            .filter(Boolean),
        }),
      }),
    onSuccess: (saved, submitted) => {
      queryClient.setQueryData(["site-tls"], saved);
      tlsDraft.saved(submitted);
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
  if (!user) return null;
  if (
    (credentialsQuery.isError && !credentialsQuery.data) ||
    (siteTLSQuery.isError && !siteTLSQuery.data) ||
    (localStorageQuery.isError && !localStorageQuery.data) ||
    (cleanupCandidatesQuery.isError && !cleanupCandidatesQuery.data)
  )
    return (
      <PageStatus
        retry={() => {
          void credentialsQuery.refetch();
          void siteTLSQuery.refetch();
          void localStorageQuery.refetch();
          void cleanupCandidatesQuery.refetch();
        }}
      />
    );
  if (
    credentialsQuery.isLoading ||
    siteTLSQuery.isLoading ||
    localStorageQuery.isLoading ||
    cleanupCandidatesQuery.isLoading
  )
    return <PageStatus loading />;
  return (
    <div className="feature-page space-y-6">
      {(credentialsQuery.isError ||
        siteTLSQuery.isError ||
        localStorageQuery.isError ||
        cleanupCandidatesQuery.isError) && (
        <p role="alert">
          {language === "zh"
            ? "后台刷新失败，当前显示上次数据；未保存的修改已保留。"
            : "Background refresh failed. Showing previous data; unsaved edits are preserved."}
        </p>
      )}
      <UnsavedChangesGuard
        dirty={
          storageDraft.dirty ||
          tlsDraft.dirty ||
          tlsCredentialSecret !== '{"secret_id":"","secret_key":""}'
        }
      />
      <div className="grid gap-4">
        <DraftFeedback
          title={ui.siteTLS}
          dirty={tlsDraft.dirty}
          saved={saveSiteTLSMutation.isSuccess}
          pending={saveSiteTLSMutation.isPending}
          onDiscard={() => {
            tlsDraft.discard();
            saveSiteTLSMutation.reset();
          }}
        />
        <SiteTLSPanel
          credentials={credentials.filter(
            (item) =>
              item.scope === "SYSTEM" &&
              item.platform === "tencent_ssl" &&
              item.purpose === "TLS",
          )}
          credentialCreateError={createTLSCredentialMutation.isError}
          credentialCreatePending={createTLSCredentialMutation.isPending}
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
          onSave={() => saveSiteTLSMutation.mutate(siteTLSForm)}
          onSync={() => syncSiteTLSMutation.mutate()}
        />
        <DraftFeedback
          title={ui.storageSettings}
          dirty={storageDraft.dirty}
          saved={saveStorageSettingsMutation.isSuccess}
          pending={saveStorageSettingsMutation.isPending}
          onDiscard={() => {
            storageDraft.discard();
            saveStorageSettingsMutation.reset();
          }}
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
          onSave={() => saveStorageSettingsMutation.mutate(storageForm)}
        />
      </div>
    </div>
  );
}
