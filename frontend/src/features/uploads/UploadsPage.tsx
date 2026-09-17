import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import {
  type CredentialForm,
  type UploadSettingsForm,
  type MeResponse,
  type ProfileListResponse,
  type CredentialListResponse,
  type BilibiliPublishingConfig,
  type COSStorageConfig,
  type Credential,
  type UploadModuleReconcileResult,
} from "../../shared/console/types";
import {
  emptyCredentialForm,
  emptyUploadSettingsForm,
  bilibiliSettingsFromConfig,
  parseConfigJSON,
  bilibiliSettingsPayload,
} from "./model";
import { requestJson } from "../../shared/api/client";
import {
  hasManagerPermission,
  bytesToGB,
  gbToBytes,
} from "../../shared/console/format";
import { uiCopy } from "../../shared/console/copy";
import { UploadSettingsPanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function UploadsPage() {
  const language = useLanguage();
  const queryClient = useQueryClient();
  const [credentialForm, setCredentialForm] =
    useState<CredentialForm>(emptyCredentialForm);
  const [uploadSettingsForm, setUploadSettingsForm] =
    useState<UploadSettingsForm>(emptyUploadSettingsForm);
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
  const profilesQuery = useQuery({
    queryKey: ["recording-profiles"],
    queryFn: () =>
      requestJson<ProfileListResponse>("/api/v1/recording-profiles"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 10000,
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
  const profiles = useMemo(
    () => profilesQuery.data?.items ?? [],
    [profilesQuery.data?.items],
  );
  const uploadProfileId = Number(uploadSettingsForm.profile_id);
  const uploadProfile = profiles.find(
    (profile) => profile.id === uploadProfileId,
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
  if (!user) return null;
  if (
    profilesQuery.isError ||
    credentialsQuery.isError ||
    bilibiliConfigQuery.isError ||
    cosConfigQuery.isError
  )
    return (
      <PageStatus
        retry={() => {
          void profilesQuery.refetch();
          void credentialsQuery.refetch();
          void bilibiliConfigQuery.refetch();
          void cosConfigQuery.refetch();
        }}
      />
    );
  if (
    profilesQuery.isLoading ||
    credentialsQuery.isLoading ||
    bilibiliConfigQuery.isLoading ||
    cosConfigQuery.isLoading
  )
    return <PageStatus loading />;
  return (
    <div className="feature-page space-y-6">
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
    </div>
  );
}
