import { RefreshWarning } from "../../shared/ui/RefreshWarning";
import { useServerDraft } from "../../shared/forms/useServerDraft";
import { UnsavedChangesGuard } from "../../shared/forms/UnsavedChangesGuard";
import { DraftFeedback } from "../../shared/forms/DraftFeedback";
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
  cosSettingsFromConfig,
} from "./model";
import { requestJson } from "../../shared/api/client";
import { hasManagerPermission, gbToBytes } from "../../shared/console/format";
import { uiCopy } from "../../shared/console/copy";
import { UploadSettingsPanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function UploadsPage() {
  const language = useLanguage();
  const queryClient = useQueryClient();
  const [credentialForm, setCredentialForm] =
    useState<CredentialForm>(emptyCredentialForm);
  const [selectedProfileId, setSelectedProfileId] = useState("");
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
  const uploadProfileId = Number(
    selectedProfileId ||
      (profiles.find((p) => !p.archived_at) ?? profiles[0])?.id ||
      0,
  );
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
  const bilibiliConfig = bilibiliConfigQuery.data;
  const bilibiliSettings = bilibiliSettingsFromConfig(
    bilibiliConfig?.settings ?? {},
  );
  const bilibiliDraft = useServerDraft({
    bilibili_enabled: bilibiliConfig?.enabled ?? false,
    bilibili_credential_id: bilibiliConfig?.credential_id
      ? String(bilibiliConfig.credential_id)
      : "",
    bilibili_title_template: bilibiliSettings.title_template,
    bilibili_description_template: bilibiliSettings.description_template,
    bilibili_tags: bilibiliSettings.tags,
    bilibili_copyright: bilibiliSettings.copyright,
    bilibili_source: bilibiliSettings.source,
    bilibili_upload_limit: bilibiliSettings.upload_limit,
  });
  const cosDraft = useServerDraft(cosSettingsFromConfig(cosConfigQuery.data));
  useEffect(() => {
    if (!selectedProfileId && uploadProfileId)
      setSelectedProfileId(String(uploadProfileId));
  }, [selectedProfileId, uploadProfileId]);
  const uploadSettingsForm: UploadSettingsForm = {
    ...emptyUploadSettingsForm,
    profile_id: String(uploadProfileId || ""),
    ...bilibiliDraft.form,
    ...cosDraft.form,
  };
  const credentialDirty =
    JSON.stringify(credentialForm) !== JSON.stringify(emptyCredentialForm);
  const settingsDirty = bilibiliDraft.dirty || cosDraft.dirty;
  const createCredentialMutation = useMutation({
    mutationFn: (submitted: CredentialForm) => {
      const secret = parseConfigJSON(submitted.secret);
      return requestJson<Credential>("/api/v1/credentials", {
        method: "POST",
        body: JSON.stringify({
          scope: "USER",
          platform: submitted.platform,
          purpose: submitted.platform === "bilibili" ? "PUBLISHER" : "STORAGE",
          account_label: submitted.account_label,
          external_uid: submitted.external_uid,
          secret,
        }),
      });
    },
    onSuccess: (_saved, submitted) => {
      setCredentialForm((current) =>
        current === submitted ? emptyCredentialForm : current,
      );
      void queryClient.invalidateQueries({ queryKey: ["credentials"] });
    },
  });
  const saveBilibiliConfigMutation = useMutation({
    mutationFn: (submitted: {
      profileId: number;
      form: typeof bilibiliDraft.form;
    }) =>
      requestJson<BilibiliPublishingConfig>(
        `/api/v1/recording-profiles/${submitted.profileId}/publishing/bilibili`,
        {
          method: "PUT",
          body: JSON.stringify({
            enabled: submitted.form.bilibili_enabled,
            credential_id: Number(submitted.form.bilibili_credential_id || 0),
            settings: bilibiliSettingsPayload({
              ...emptyUploadSettingsForm,
              ...submitted.form,
            }),
          }),
        },
      ),
    onSuccess: (saved, submitted) => {
      queryClient.setQueryData(
        ["upload-config", "bilibili", submitted.profileId],
        saved,
      );
      bilibiliDraft.saved(submitted.form);
      void queryClient.invalidateQueries({
        queryKey: ["upload-config", "bilibili", submitted.profileId],
      });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
  const saveCosConfigMutation = useMutation({
    mutationFn: (submitted: {
      profileId: number;
      form: typeof cosDraft.form;
    }) =>
      requestJson<COSStorageConfig>(
        `/api/v1/recording-profiles/${submitted.profileId}/storage/cos`,
        {
          method: "PUT",
          body: JSON.stringify({
            enabled: submitted.form.cos_enabled,
            credential_id: Number(submitted.form.cos_credential_id || 0),
            region: submitted.form.cos_region,
            bucket: submitted.form.cos_bucket,
            prefix: submitted.form.cos_prefix,
            max_managed_bytes: gbToBytes(submitted.form.cos_max_managed_gb),
          }),
        },
      ),
    onSuccess: (saved, submitted) => {
      queryClient.setQueryData(
        ["upload-config", "cos", submitted.profileId],
        saved,
      );
      if (
        saved.enabled ||
        JSON.stringify(submitted.form) ===
          JSON.stringify(cosSettingsFromConfig(saved))
      ) {
        cosDraft.saved(submitted.form);
      }
      void queryClient.invalidateQueries({
        queryKey: ["upload-config", "cos", submitted.profileId],
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
  const setUploadSettingsForm = (next: UploadSettingsForm) => {
    if (next.profile_id !== uploadSettingsForm.profile_id) {
      if (
        saveBilibiliConfigMutation.isPending ||
        saveCosConfigMutation.isPending
      )
        return;
      if (
        settingsDirty &&
        !window.confirm(
          language === "zh"
            ? "切换配置将放弃未保存的上传设置，继续？"
            : "Discard unsaved upload settings and switch profile?",
        )
      )
        return;
      bilibiliDraft.discard();
      cosDraft.discard();
      saveBilibiliConfigMutation.reset();
      saveCosConfigMutation.reset();
      setSelectedProfileId(next.profile_id);
      return;
    }
    const bili = Object.fromEntries(
      Object.keys(bilibiliDraft.form).map((key) => [
        key,
        next[key as keyof UploadSettingsForm],
      ]),
    ) as typeof bilibiliDraft.form;
    const cos = Object.fromEntries(
      Object.keys(cosDraft.form).map((key) => [
        key,
        next[key as keyof UploadSettingsForm],
      ]),
    ) as typeof cosDraft.form;
    if (JSON.stringify(bili) !== JSON.stringify(bilibiliDraft.form))
      bilibiliDraft.change(bili);
    if (JSON.stringify(cos) !== JSON.stringify(cosDraft.form))
      cosDraft.change(cos);
  };
  if (!user) return null;
  if (
    (profilesQuery.isError && !profilesQuery.data) ||
    (credentialsQuery.isError && !credentialsQuery.data) ||
    (bilibiliConfigQuery.isError && !bilibiliConfigQuery.data) ||
    (cosConfigQuery.isError && !cosConfigQuery.data)
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
      <RefreshWarning
        failed={[
          profilesQuery,
          credentialsQuery,
          bilibiliConfigQuery,
          cosConfigQuery,
        ].some((query) => query.isError)}
        retry={() =>
          [profilesQuery, credentialsQuery, bilibiliConfigQuery, cosConfigQuery]
            .filter((query) => query.isError)
            .forEach((query) => {
              void query.refetch();
            })
        }
      />
      <UnsavedChangesGuard dirty={settingsDirty || credentialDirty} />
      <DraftFeedback
        title={ui.bilibiliPublishing}
        dirty={bilibiliDraft.dirty}
        saved={saveBilibiliConfigMutation.isSuccess}
        pending={saveBilibiliConfigMutation.isPending}
        onDiscard={() => {
          bilibiliDraft.discard();
          saveBilibiliConfigMutation.reset();
        }}
      />
      <DraftFeedback
        title={ui.cosStorage}
        dirty={cosDraft.dirty}
        saved={saveCosConfigMutation.isSuccess}
        pending={saveCosConfigMutation.isPending}
        onDiscard={() => {
          cosDraft.discard();
          saveCosConfigMutation.reset();
        }}
      />
      {saveCosConfigMutation.isSuccess &&
        !saveCosConfigMutation.data.enabled && (
          <p role="status" className="text-sm text-muted">
            {language === "zh"
              ? "COS 已禁用。禁用操作不保存其他配置字段；这些修改仍保留为草稿。"
              : "COS is disabled. Disabling does not save other fields; those edits remain a draft."}
          </p>
        )}
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
          createCredentialMutation.mutate(credentialForm);
        }}
        onReconcile={() => reconcileUploadModulesMutation.mutate()}
        onSaveBilibiliConfig={() =>
          saveBilibiliConfigMutation.mutate({
            profileId: uploadProfileId,
            form: bilibiliDraft.form,
          })
        }
        onSaveCosConfig={() =>
          saveCosConfigMutation.mutate({
            profileId: uploadProfileId,
            form: cosDraft.form,
          })
        }
        onSettingsFormChange={setUploadSettingsForm}
      />
    </div>
  );
}
