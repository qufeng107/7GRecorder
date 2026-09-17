import {
  type Credential,
  type SiteTLSForm,
  type AdminCopy,
  type SiteTLSSettings,
  type CleanupCandidate,
  type CleanupRunResult,
  type LocalStorageStatus,
} from "../../shared/console/types";
import { type FormEvent } from "react";
import {
  HardDrive,
  Lock,
  RefreshCw,
  Save,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import {
  ToggleField,
  CredentialSelect,
  TextField,
  TextAreaField,
  Metric,
  JSONTextArea,
  NumberField,
} from "../../shared/console/fields";
import { formatDateTime, formatBytes } from "../../shared/console/format";

export function SiteTLSPanel(props: {
  credentialCreateError: boolean;
  credentialCreatePending: boolean;
  credentialLabel: string;
  credentialSecret: string;
  credentials: Credential[];
  form: SiteTLSForm;
  labels: AdminCopy;
  saveError: boolean;
  savePending: boolean;
  settings?: SiteTLSSettings;
  syncPending: boolean;
  onCreateCredential: (event: FormEvent<HTMLFormElement>) => void;
  onCredentialLabelChange: (value: string) => void;
  onCredentialSecretChange: (value: string) => void;
  onFormChange: (form: SiteTLSForm) => void;
  onSave: () => void;
  onSync: () => void;
}) {
  const update = <K extends keyof SiteTLSForm>(
    key: K,
    value: SiteTLSForm[K],
  ) => {
    props.onFormChange({ ...props.form, [key]: value });
  };

  return (
    <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex items-start gap-3">
        <ShieldCheck
          className="mt-0.5 h-5 w-5 text-accent"
          aria-hidden="true"
        />
        <div>
          <h2 className="text-sm font-semibold">{props.labels.siteTLS}</h2>
          <p className="mt-1 text-sm text-muted">{props.labels.siteTLSHint}</p>
        </div>
      </div>
      <div className="mt-4 grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.8fr)]">
        <div className="grid content-start gap-3">
          <ToggleField
            label={props.labels.enabled}
            checked={props.form.enabled}
            onChange={(value) => update("enabled", value)}
          />
          <CredentialSelect
            credentials={props.credentials}
            label={props.labels.tlsCredential}
            labels={props.labels}
            value={props.form.credential_id}
            onChange={(value) => update("credential_id", value)}
          />
          <TextField
            disabled
            label={props.labels.primaryDomain}
            value={props.form.primary_domain}
            onChange={(value) => update("primary_domain", value)}
          />
          <TextAreaField
            disabled
            label={props.labels.additionalDomains}
            value={props.form.additional_domains}
            onChange={(value) => update("additional_domains", value)}
          />
          <p className="text-xs text-muted">
            {props.labels.additionalDomainsHint}
          </p>
          {props.saveError ? (
            <p className="text-sm text-red-700">
              {props.labels.siteTLSSaveFailed}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <button
              className="inline-flex h-9 items-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
              disabled={props.savePending}
              type="button"
              onClick={props.onSave}
            >
              <Save className="h-4 w-4" aria-hidden="true" />
              {props.labels.saveSiteTLS}
            </button>
            <button
              className="inline-flex h-9 items-center gap-2 rounded-md border border-border bg-white px-3 text-sm font-semibold disabled:opacity-60"
              disabled={props.syncPending || !props.settings?.enabled}
              type="button"
              onClick={props.onSync}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              {props.labels.syncSiteTLS}
            </button>
          </div>
        </div>
        <div className="grid content-start gap-4 border-t border-border pt-4 lg:border-l lg:border-t-0 lg:pl-5 lg:pt-0">
          <div className="grid grid-cols-2 gap-3">
            <Metric
              label={props.labels.siteTLSStatus}
              value={props.settings?.status ?? props.labels.unknown}
            />
            <Metric
              label={props.labels.latestCertificate}
              value={props.settings?.latest_certificate_id ?? "-"}
            />
            <Metric
              label={props.labels.stagedCertificate}
              value={props.settings?.staged_certificate_id ?? "-"}
            />
            <Metric
              label={props.labels.deployedCertificate}
              value={props.settings?.deployed_certificate_id ?? "-"}
            />
            <Metric
              label={props.labels.certificateExpires}
              value={
                props.settings?.latest_not_after
                  ? formatDateTime(
                      props.settings.latest_not_after,
                      props.labels,
                    )
                  : "-"
              }
            />
            <Metric
              label={props.labels.lastChecked}
              value={
                props.settings?.last_checked_at
                  ? formatDateTime(props.settings.last_checked_at, props.labels)
                  : "-"
              }
            />
          </div>
          {props.settings?.last_error ? (
            <p className="text-sm text-red-700">{props.settings.last_error}</p>
          ) : null}
          <form
            className="grid gap-3 border-t border-border pt-4"
            onSubmit={props.onCreateCredential}
          >
            <h3 className="text-sm font-semibold">
              {props.labels.tlsCredential}
            </h3>
            <TextField
              label={props.labels.tlsCredentialLabel}
              value={props.credentialLabel}
              onChange={props.onCredentialLabelChange}
            />
            <JSONTextArea
              label={props.labels.tlsCredentialSecret}
              value={props.credentialSecret}
              onChange={props.onCredentialSecretChange}
            />
            <p className="text-xs text-muted">
              {props.labels.credentialSecretHint}
            </p>
            {props.credentialCreateError ? (
              <p className="text-sm text-red-700">
                {props.labels.tlsCredentialCreateFailed}
              </p>
            ) : null}
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border bg-white px-3 text-sm font-semibold disabled:opacity-60"
              disabled={props.credentialCreatePending}
              type="submit"
            >
              <Lock className="h-4 w-4" aria-hidden="true" />
              {props.labels.createTLSCredential}
            </button>
          </form>
        </div>
      </div>
    </section>
  );
}

export function StoragePanel(props: {
  candidates: CleanupCandidate[];
  cleanupError: boolean;
  cleanupPending: boolean;
  cleanupResult?: CleanupRunResult;
  form: {
    maxRecordingGB: number;
    minFreeGB: number;
    emergencyFreeGB: number;
    cleanupTargetPercent: number;
  };
  isLoading: boolean;
  isSaving: boolean;
  labels: AdminCopy;
  previewReclaimableBytes: number;
  saveError: boolean;
  status?: LocalStorageStatus;
  onFormChange: (form: {
    maxRecordingGB: number;
    minFreeGB: number;
    emergencyFreeGB: number;
    cleanupTargetPercent: number;
  }) => void;
  onRunCleanup: () => void;
  onSave: () => void;
}) {
  const usedPercent =
    props.status && props.status.disk_total_bytes > 0
      ? Math.round(
          ((props.status.disk_total_bytes - props.status.disk_available_bytes) /
            props.status.disk_total_bytes) *
            100,
        )
      : 0;
  const update = <K extends keyof typeof props.form>(
    key: K,
    value: (typeof props.form)[K],
  ) => {
    props.onFormChange({ ...props.form, [key]: value });
  };

  return (
    <section
      id="storage"
      className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{props.labels.localStorage}</h2>
          <p className="mt-1 text-sm text-muted">
            {props.status?.data_root ?? props.labels.checkingStorage}
          </p>
        </div>
        <HardDrive
          className="h-5 w-5 shrink-0 text-accent"
          aria-hidden="true"
        />
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <Metric
          label={props.labels.indexedVideos}
          value={
            props.isLoading
              ? "..."
              : String(props.status?.indexed_video_files ?? 0)
          }
        />
        <Metric
          label={props.labels.indexedSize}
          value={formatBytes(props.status?.indexed_video_bytes ?? 0)}
        />
        <Metric
          label={props.labels.diskAvailable}
          value={formatBytes(props.status?.disk_available_bytes ?? 0)}
        />
        <Metric
          label={props.labels.protected}
          value={String(props.status?.protected_recordings ?? 0)}
        />
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <Metric
          label={props.labels.health}
          value={props.status?.health ?? props.labels.checking}
        />
        <Metric
          label={props.labels.needReclaim}
          value={formatBytes(props.status?.need_reclaim_bytes ?? 0)}
        />
        <Metric
          label={props.labels.previewReclaimable}
          value={formatBytes(props.previewReclaimableBytes)}
        />
      </div>

      <div className="mt-4 h-2 overflow-hidden rounded-full bg-[#e6ebe4]">
        <div
          className="h-full bg-accent"
          style={{ width: `${Math.min(100, Math.max(0, usedPercent))}%` }}
        />
      </div>
      <p className="mt-2 text-xs text-muted">
        {props.labels.diskSummary(
          usedPercent,
          formatBytes(props.status?.disk_total_bytes ?? 0),
          props.status?.completed_recordings ?? 0,
          Boolean(props.status?.settings_configured),
        )}
      </p>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">
            {props.labels.storageSettings}
          </h3>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.isSaving}
            type="button"
            onClick={props.onSave}
          >
            <Save className="h-4 w-4" aria-hidden="true" />
            {props.labels.save}
          </button>
        </div>
        <div className="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <NumberField
            label={props.labels.maxRecordingGB}
            min={1}
            value={props.form.maxRecordingGB}
            onChange={(value) => update("maxRecordingGB", value)}
          />
          <NumberField
            label={props.labels.minFreeGB}
            min={1}
            value={props.form.minFreeGB}
            onChange={(value) => update("minFreeGB", value)}
          />
          <NumberField
            label={props.labels.emergencyFreeGB}
            min={1}
            value={props.form.emergencyFreeGB}
            onChange={(value) => update("emergencyFreeGB", value)}
          />
          <NumberField
            label={props.labels.cleanupTargetPercent}
            max={99}
            min={1}
            value={props.form.cleanupTargetPercent}
            onChange={(value) => update("cleanupTargetPercent", value)}
          />
        </div>
        {props.saveError ? (
          <p className="mt-3 text-sm text-red-700">
            {props.labels.storageSaveFailed}
          </p>
        ) : null}
      </div>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">
              {props.labels.cleanupPreview}
            </h3>
            <span className="text-xs text-muted">
              {props.labels.oldestUnprotected}
            </span>
          </div>
          <button
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-red-300 px-3 text-sm font-semibold text-red-700 hover:border-red-500 disabled:opacity-50"
            disabled={
              props.cleanupPending ||
              props.candidates.length === 0 ||
              (props.status?.need_reclaim_bytes ?? 0) <= 0
            }
            type="button"
            onClick={props.onRunCleanup}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            {props.labels.runCleanup}
          </button>
        </div>
        {props.cleanupResult ? (
          <p className="mt-3 text-sm text-muted">
            {props.labels.cleanupResult(
              props.cleanupResult.deleted_recordings,
              props.cleanupResult.deleted_files,
              formatBytes(props.cleanupResult.reclaimed_bytes),
              props.cleanupResult.skipped_recordings,
            )}
          </p>
        ) : null}
        {props.cleanupError ? (
          <p className="mt-3 text-sm text-red-700">
            {props.labels.cleanupFailed}
          </p>
        ) : null}
        <div className="mt-3 overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.recording}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.closed}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.files}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.reclaimable}
                </th>
              </tr>
            </thead>
            <tbody>
              {props.candidates.map((candidate) => (
                <tr key={candidate.recording_id} className="bg-white">
                  <td className="px-3 py-3">
                    <p className="font-medium text-ink">
                      {candidate.title ||
                        candidate.streamer_name ||
                        props.labels.untitled}
                    </p>
                    <p className="mt-1 text-xs text-muted">
                      {candidate.profile_name} - {candidate.room_id}
                    </p>
                  </td>
                  <td className="px-3 py-3 text-xs text-muted">
                    {formatDateTime(candidate.completed_at || "", props.labels)}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {candidate.file_count}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatBytes(candidate.reclaimable_bytes)}
                  </td>
                </tr>
              ))}
              {props.candidates.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-muted" colSpan={4}>
                    {props.labels.noCleanupCandidates}
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
