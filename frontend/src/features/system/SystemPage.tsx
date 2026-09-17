import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
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
  const [storageForm, setStorageForm] = useState({
    maxRecordingGB: 0,
    minFreeGB: 0,
    emergencyFreeGB: 0,
    cleanupTargetPercent: 85,
  });
  const [siteTLSForm, setSiteTLSForm] = useState<SiteTLSForm>(emptySiteTLSForm);
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
  if (!user) return null;
  if (
    credentialsQuery.isError ||
    siteTLSQuery.isError ||
    localStorageQuery.isError ||
    cleanupCandidatesQuery.isError
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
      <div className="grid gap-4">
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
    </div>
  );
}
