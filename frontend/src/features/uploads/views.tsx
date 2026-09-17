import {
  type CredentialForm,
  type Credential,
  type AdminCopy,
  type RecordingProfile,
  type UploadModuleReconcileResult,
  type UploadSettingsForm,
} from "../../shared/console/types";
import { type FormEvent } from "react";
import { CloudUpload, HardDrive, RefreshCw, Save } from "lucide-react";
import {
  ToggleField,
  CredentialSelect,
  TextField,
  TextAreaField,
  NumberField,
  JSONTextArea,
} from "../../shared/console/fields";

export function UploadSettingsPanel(props: {
  bilibiliConfigError: boolean;
  bilibiliConfigPending: boolean;
  canEditBilibiliModule: boolean;
  canEditCosModule: boolean;
  canReconcileUploadJobs: boolean;
  cosConfigError: boolean;
  cosConfigPending: boolean;
  credentialCreateError: boolean;
  credentialCreatePending: boolean;
  credentialForm: CredentialForm;
  credentials: Credential[];
  labels: AdminCopy;
  profiles: RecordingProfile[];
  reconcileError: boolean;
  reconcilePending: boolean;
  reconcileResult?: UploadModuleReconcileResult;
  selectedProfile?: RecordingProfile;
  settingsForm: UploadSettingsForm;
  onCreateCredential: (event: FormEvent<HTMLFormElement>) => void;
  onCredentialFormChange: (form: CredentialForm) => void;
  onReconcile: () => void;
  onSaveBilibiliConfig: () => void;
  onSaveCosConfig: () => void;
  onSettingsFormChange: (form: UploadSettingsForm) => void;
}) {
  const updateCredential = <K extends keyof CredentialForm>(
    key: K,
    value: CredentialForm[K],
  ) => {
    props.onCredentialFormChange({ ...props.credentialForm, [key]: value });
  };
  const updateSettings = <K extends keyof UploadSettingsForm>(
    key: K,
    value: UploadSettingsForm[K],
  ) => {
    props.onSettingsFormChange({ ...props.settingsForm, [key]: value });
  };
  const bilibiliCredentials = props.credentials.filter(
    (credential) =>
      credential.platform === "bilibili" && credential.purpose === "PUBLISHER",
  );
  const cosCredentials = props.credentials.filter(
    (credential) =>
      credential.platform === "tencent_cos" && credential.purpose === "STORAGE",
  );
  const selectedProfileMissing =
    props.settingsForm.profile_id &&
    !props.profiles.some(
      (profile) => String(profile.id) === props.settingsForm.profile_id,
    );

  if (!props.canEditBilibiliModule && !props.canEditCosModule) {
    return (
      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.uploadSettings}</h2>
        <p className="mt-3 text-sm text-muted">
          {props.labels.uploadAccessBlocked}
        </p>
      </section>
    );
  }

  return (
    <section
      id="uploads"
      className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_380px]"
    >
      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">
              {props.labels.uploadSettings}
            </h2>
            <p className="mt-1 text-sm text-muted">
              {props.labels.uploadProfileHint}
            </p>
          </div>
          {props.canReconcileUploadJobs ? (
            <div className="flex flex-col items-end gap-1">
              <button
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
                disabled={props.reconcilePending}
                type="button"
                onClick={props.onReconcile}
              >
                <RefreshCw className="h-4 w-4" aria-hidden="true" />
                {props.labels.reconcileUploadJobs}
              </button>
              <p className="max-w-xs text-right text-xs text-muted">
                {props.labels.reconcileUploadHint}
              </p>
            </div>
          ) : null}
        </div>

        {props.reconcileResult ? (
          <p className="mt-3 text-sm text-muted">
            {props.labels.uploadReconcileResult(
              props.reconcileResult.publications_created,
              props.reconcileResult.bilibili_jobs_created,
              props.reconcileResult.cos_objects_created,
              props.reconcileResult.cos_jobs_created,
              props.reconcileResult.cos_file_objects_created ?? 0,
              props.reconcileResult.cos_file_jobs_created ?? 0,
            )}
          </p>
        ) : null}
        {props.reconcileError ? (
          <p className="mt-3 text-sm text-red-700">
            {props.labels.uploadReconcileFailed}
          </p>
        ) : null}

        <label className="mt-4 flex flex-col gap-1 text-sm font-medium">
          {props.labels.uploadProfile}
          <select
            className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
            value={props.settingsForm.profile_id}
            onChange={(event) =>
              updateSettings("profile_id", event.target.value)
            }
          >
            {selectedProfileMissing ? (
              <option value={props.settingsForm.profile_id}>
                {props.labels.currentOwner}
              </option>
            ) : null}
            {props.profiles.map((profile) => (
              <option key={profile.id} value={profile.id}>
                {profile.name} - {profile.room_id}
              </option>
            ))}
          </select>
        </label>

        <div className="mt-5 grid gap-4 xl:grid-cols-2">
          <section className="rounded-md border border-border bg-white p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold">
                  {props.labels.bilibiliPublishing}
                </h3>
                <p className="mt-1 text-xs text-muted">
                  {props.settingsForm.bilibili_enabled
                    ? props.labels.moduleEnabled
                    : props.labels.moduleDisabled}
                </p>
              </div>
              <CloudUpload className="h-5 w-5 text-accent" aria-hidden="true" />
            </div>
            <div className="mt-4 grid gap-3">
              <ToggleField
                disabled={!props.canEditBilibiliModule}
                label={props.labels.enabled}
                checked={props.settingsForm.bilibili_enabled}
                onChange={(value) => updateSettings("bilibili_enabled", value)}
              />
              <CredentialSelect
                credentials={bilibiliCredentials}
                disabled={!props.canEditBilibiliModule}
                label={props.labels.credential}
                labels={props.labels}
                value={props.settingsForm.bilibili_credential_id}
                onChange={(value) =>
                  updateSettings("bilibili_credential_id", value)
                }
              />
              <TextField
                disabled={!props.canEditBilibiliModule}
                label={props.labels.bilibiliTitleTemplate}
                value={props.settingsForm.bilibili_title_template}
                onChange={(value) =>
                  updateSettings("bilibili_title_template", value)
                }
              />
              <TextAreaField
                disabled={!props.canEditBilibiliModule}
                label={props.labels.bilibiliDescriptionTemplate}
                value={props.settingsForm.bilibili_description_template}
                onChange={(value) =>
                  updateSettings("bilibili_description_template", value)
                }
              />
              <TextField
                disabled={!props.canEditBilibiliModule}
                label={props.labels.bilibiliTags}
                value={props.settingsForm.bilibili_tags}
                onChange={(value) => updateSettings("bilibili_tags", value)}
              />
              <label className="flex flex-col gap-1 text-sm font-medium">
                {props.labels.bilibiliCopyright}
                <select
                  className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent disabled:bg-[#f3f4f1]"
                  disabled={!props.canEditBilibiliModule}
                  value={props.settingsForm.bilibili_copyright}
                  onChange={(event) =>
                    updateSettings(
                      "bilibili_copyright",
                      Number(event.target.value),
                    )
                  }
                >
                  <option value={1}>
                    {props.labels.bilibiliCopyrightOriginal}
                  </option>
                  <option value={2}>
                    {props.labels.bilibiliCopyrightRepost}
                  </option>
                </select>
              </label>
              {props.settingsForm.bilibili_copyright === 2 ? (
                <TextField
                  disabled={!props.canEditBilibiliModule}
                  label={props.labels.bilibiliSource}
                  value={props.settingsForm.bilibili_source}
                  onChange={(value) => updateSettings("bilibili_source", value)}
                />
              ) : null}
              <label className="flex flex-col gap-1 text-sm font-medium">
                {props.labels.bilibiliUploadLimit}
                <input
                  className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent disabled:bg-[#f3f4f1]"
                  disabled={!props.canEditBilibiliModule}
                  min={1}
                  max={8}
                  type="number"
                  value={props.settingsForm.bilibili_upload_limit}
                  onChange={(event) =>
                    updateSettings(
                      "bilibili_upload_limit",
                      Number(event.target.value),
                    )
                  }
                />
              </label>
              <p className="text-xs leading-5 text-muted">
                {props.labels.bilibiliTemplateHint}
              </p>
              {props.bilibiliConfigError ? (
                <p className="text-sm text-red-700">
                  {props.labels.uploadConfigSaveFailed}
                </p>
              ) : null}
              <button
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
                disabled={
                  !props.canEditBilibiliModule ||
                  props.bilibiliConfigPending ||
                  !props.selectedProfile
                }
                type="button"
                onClick={props.onSaveBilibiliConfig}
              >
                <Save className="h-4 w-4" aria-hidden="true" />
                {props.labels.saveBilibiliConfig}
              </button>
            </div>
          </section>

          <section className="rounded-md border border-border bg-white p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold">
                  {props.labels.cosStorage}
                </h3>
                <p className="mt-1 text-xs text-muted">
                  {props.settingsForm.cos_enabled
                    ? props.labels.moduleEnabled
                    : props.labels.moduleDisabled}
                </p>
              </div>
              <HardDrive className="h-5 w-5 text-accent" aria-hidden="true" />
            </div>
            <div className="mt-4 grid gap-3">
              <ToggleField
                disabled={!props.canEditCosModule}
                label={props.labels.enabled}
                checked={props.settingsForm.cos_enabled}
                onChange={(value) => updateSettings("cos_enabled", value)}
              />
              <CredentialSelect
                credentials={cosCredentials}
                disabled={!props.canEditCosModule}
                label={props.labels.credential}
                labels={props.labels}
                value={props.settingsForm.cos_credential_id}
                onChange={(value) => updateSettings("cos_credential_id", value)}
              />
              <TextField
                label={props.labels.cosRegion}
                value={props.settingsForm.cos_region}
                onChange={(value) => updateSettings("cos_region", value)}
              />
              <TextField
                label={props.labels.cosBucket}
                value={props.settingsForm.cos_bucket}
                onChange={(value) => updateSettings("cos_bucket", value)}
              />
              <TextField
                label={props.labels.cosPrefix}
                value={props.settingsForm.cos_prefix}
                onChange={(value) => updateSettings("cos_prefix", value)}
              />
              <NumberField
                label={props.labels.cosMaxManagedGB}
                min={1}
                value={props.settingsForm.cos_max_managed_gb}
                onChange={(value) =>
                  updateSettings("cos_max_managed_gb", value)
                }
              />
              {props.cosConfigError ? (
                <p className="text-sm text-red-700">
                  {props.labels.uploadConfigSaveFailed}
                </p>
              ) : null}
              <button
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
                disabled={
                  !props.canEditCosModule ||
                  props.cosConfigPending ||
                  !props.selectedProfile
                }
                type="button"
                onClick={props.onSaveCosConfig}
              >
                <Save className="h-4 w-4" aria-hidden="true" />
                {props.labels.saveCosConfig}
              </button>
            </div>
          </section>
        </div>
      </section>

      <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">
          {props.labels.credentialVault}
        </h2>
        <div className="mt-3 grid gap-2">
          {props.credentials.map((credential) => (
            <div
              key={credential.id}
              className="rounded-md border border-border bg-white px-3 py-2"
            >
              <p className="text-sm font-semibold">
                {credential.account_label}
              </p>
              <p className="mt-1 text-xs text-muted">
                {credential.platform} / {credential.purpose} /{" "}
                {credential.status}
              </p>
            </div>
          ))}
          {props.credentials.length === 0 ? (
            <p className="text-sm text-muted">{props.labels.noCredentials}</p>
          ) : null}
        </div>

        <form
          className="mt-5 grid gap-3 border-t border-border pt-4"
          onSubmit={props.onCreateCredential}
        >
          <h3 className="text-sm font-semibold">
            {props.labels.newCredential}
          </h3>
          <label className="flex flex-col gap-1 text-sm font-medium">
            {props.labels.platform}
            <select
              className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
              value={props.credentialForm.platform}
              onChange={(event) =>
                updateCredential(
                  "platform",
                  event.target.value as CredentialForm["platform"],
                )
              }
            >
              <option value="bilibili">Bilibili</option>
              <option value="tencent_cos">Tencent COS</option>
            </select>
          </label>
          <TextField
            label={props.labels.accountLabel}
            value={props.credentialForm.account_label}
            onChange={(value) => updateCredential("account_label", value)}
          />
          <TextField
            label={props.labels.externalUid}
            value={props.credentialForm.external_uid}
            onChange={(value) => updateCredential("external_uid", value)}
          />
          <JSONTextArea
            label={props.labels.credentialSecret}
            value={props.credentialForm.secret}
            onChange={(value) => updateCredential("secret", value)}
          />
          <p className="text-xs text-muted">
            {props.labels.credentialSecretHint}
          </p>
          {props.credentialCreateError ? (
            <p className="text-sm text-red-700">
              {props.labels.credentialCreateFailed}
            </p>
          ) : null}
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.credentialCreatePending}
            type="submit"
          >
            <Save className="h-4 w-4" aria-hidden="true" />
            {props.labels.createCredential}
          </button>
        </form>
      </section>
    </section>
  );
}
