import {
  type Credential,
  type SongSettingsForm,
  type AdminCopy,
  type SongAnalysisRun,
  type RecognizedSong,
  type SongAnalysisSource,
} from "../../shared/console/types";
import { type FormEvent } from "react";
import { Lock, Music2, Save } from "lucide-react";
import { ToggleField, TextField } from "../../shared/console/fields";
import { formatBytes, formatChinaDateParts } from "../../shared/console/format";
import { formatSongOffset } from "./model";

export function SongsPanel(props: {
  acrCredentials: Credential[];
  credentialCreateError: boolean;
  credentialCreatePending: boolean;
  credentialLabel: string;
  credentialSecret: string;
  form: SongSettingsForm;
  labels: AdminCopy;
  runs: SongAnalysisRun[];
  songs: RecognizedSong[];
  saveError: boolean;
  savePending: boolean;
  selectedSourceID: string;
  sources: SongAnalysisSource[];
  startError: boolean;
  startPending: boolean;
  onCreateCredential: (event: FormEvent<HTMLFormElement>) => void;
  onCredentialLabelChange: (value: string) => void;
  onCredentialSecretChange: (value: string) => void;
  onFormChange: (form: SongSettingsForm) => void;
  onSave: () => void;
  onSelectedSourceChange: (value: string) => void;
  onStart: () => void;
}) {
  const update = <K extends keyof SongSettingsForm>(
    key: K,
    value: SongSettingsForm[K],
  ) => {
    props.onFormChange({ ...props.form, [key]: value });
  };
  return (
    <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <Music2 className="h-5 w-5 text-accent" aria-hidden="true" />
        <h2 className="text-sm font-semibold">{props.labels.songsTitle}</h2>
      </div>

      <div className="mt-4 grid gap-5 lg:grid-cols-2">
        <div className="grid content-start gap-3">
          <h3 className="text-sm font-semibold">
            {props.labels.songsSettings}
          </h3>
          <ToggleField
            checked={props.form.enabled}
            label={props.labels.songsEnabled}
            onChange={(value) => update("enabled", value)}
          />
          <label className="grid gap-1 text-sm font-medium">
            {props.labels.acrCredential}
            <select
              className="h-10 rounded-md border border-border bg-white px-3"
              value={props.form.credential_id}
              onChange={(event) => update("credential_id", event.target.value)}
            >
              <option value="">{props.labels.noCredentialSelected}</option>
              {props.acrCredentials.map((credential) => (
                <option key={credential.id} value={credential.id}>
                  {credential.account_label}
                </option>
              ))}
            </select>
          </label>
          <TextField
            label={props.labels.providerRegion}
            value={props.form.region}
            onChange={(value) => update("region", value)}
          />
          <TextField
            label={props.labels.providerContainer}
            value={props.form.container_id}
            onChange={(value) => update("container_id", value)}
          />
          <TextField
            label={props.labels.destinationCosProfile}
            type="number"
            value={props.form.destination_cos_storage_profile_id}
            onChange={(value) =>
              update("destination_cos_storage_profile_id", value)
            }
          />
          <TextField
            label={props.labels.songsPrefix}
            value={props.form.songs_prefix}
            onChange={(value) => update("songs_prefix", value)}
          />
          <TextField
            label={props.labels.boundaryPadding}
            type="number"
            value={String(props.form.boundary_padding_ms)}
            onChange={(value) => update("boundary_padding_ms", Number(value))}
          />
          {props.saveError ? (
            <p className="text-sm text-red-700">
              {props.labels.songsSettingsFailed}
            </p>
          ) : null}
          <button
            className="inline-flex h-9 w-fit items-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={props.savePending}
            type="button"
            onClick={props.onSave}
          >
            <Save className="h-4 w-4" aria-hidden="true" />
            {props.labels.saveSongsSettings}
          </button>
        </div>

        <form
          className="grid content-start gap-3 border-t border-border pt-4 lg:border-l lg:border-t-0 lg:pl-5 lg:pt-0"
          onSubmit={props.onCreateCredential}
        >
          <h3 className="text-sm font-semibold">
            {props.labels.acrCredential}
          </h3>
          <TextField
            label={props.labels.acrCredentialName}
            value={props.credentialLabel}
            onChange={props.onCredentialLabelChange}
          />
          <label className="grid gap-1 text-sm font-medium">
            {props.labels.acrSecret}
            <textarea
              className="min-h-28 rounded-md border border-border bg-white p-3 font-mono text-xs"
              value={props.credentialSecret}
              onChange={(event) =>
                props.onCredentialSecretChange(event.target.value)
              }
            />
          </label>
          {props.credentialCreateError ? (
            <p className="text-sm text-red-700">
              {props.labels.songsCredentialFailed}
            </p>
          ) : null}
          <button
            className="inline-flex h-9 w-fit items-center gap-2 rounded-md border border-border px-3 text-sm font-medium disabled:opacity-60"
            disabled={props.credentialCreatePending}
            type="submit"
          >
            <Lock className="h-4 w-4" aria-hidden="true" />
            {props.labels.saveAcrCredential}
          </button>
        </form>
      </div>

      <div className="mt-5 border-t border-border pt-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid min-w-0 flex-1 gap-1 text-sm font-medium">
            {props.labels.analysisSource}
            <select
              className="h-10 min-w-0 rounded-md border border-border bg-white px-3"
              value={props.selectedSourceID}
              onChange={(event) =>
                props.onSelectedSourceChange(event.target.value)
              }
            >
              {props.sources.length === 0 ? (
                <option value="">{props.labels.noSongSources}</option>
              ) : null}
              {props.sources.map((source) => (
                <option key={source.cos_object_id} value={source.cos_object_id}>
                  {source.profile_name} · {source.object_key} ·{" "}
                  {formatBytes(source.size_bytes)}
                </option>
              ))}
            </select>
          </label>
          <button
            className="inline-flex h-10 items-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
            disabled={!props.selectedSourceID || props.startPending}
            type="button"
            onClick={props.onStart}
          >
            <Music2 className="h-4 w-4" aria-hidden="true" />
            {props.labels.startAnalysis}
          </button>
        </div>
        {props.startError ? (
          <p className="mt-2 text-sm text-red-700">
            {props.labels.startAnalysisFailed}
          </p>
        ) : null}
      </div>

      <div className="mt-5 border-t border-border pt-4">
        <h3 className="text-sm font-semibold">{props.labels.analysisRuns}</h3>
        <div className="mt-3 overflow-x-auto">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs text-muted">
              <tr>
                <th className="px-3 py-2">ID</th>
                <th className="px-3 py-2">COS</th>
                <th className="px-3 py-2">{props.labels.status}</th>
                <th className="px-3 py-2">{props.labels.analysisCreated}</th>
              </tr>
            </thead>
            <tbody>
              {props.runs.map((run) => (
                <tr
                  key={run.id}
                  className="border-b border-border last:border-0"
                >
                  <td className="px-3 py-3">{run.id}</td>
                  <td className="max-w-xl break-all px-3 py-3">
                    <p>{run.source_object_key}</p>
                    <p className="text-xs text-muted">
                      {formatBytes(run.source_size_bytes)}
                    </p>
                  </td>
                  <td className="px-3 py-3">
                    <p>{run.status}</p>
                    {run.progress_message ? (
                      <p className="text-xs text-muted">
                        {run.progress_message}
                      </p>
                    ) : null}
                    {run.last_error ? (
                      <p className="text-xs text-red-700">{run.last_error}</p>
                    ) : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatChinaDateParts(run.created_at).date}{" "}
                    {formatChinaDateParts(run.created_at).time}
                  </td>
                </tr>
              ))}
              {props.runs.length === 0 ? (
                <tr>
                  <td className="px-3 py-8 text-center text-muted" colSpan={4}>
                    {props.labels.noAnalysisRuns}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>

      <div className="mt-5 border-t border-border pt-4">
        <h3 className="text-sm font-semibold">
          {props.labels.recognizedSongs}
        </h3>
        <div className="mt-3 overflow-x-auto">
          <table className="w-full border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs text-muted">
              <tr>
                <th className="px-3 py-2">{props.labels.songAudio}</th>
                <th className="px-3 py-2">{props.labels.title}</th>
                <th className="px-3 py-2">{props.labels.songTimeRange}</th>
                <th className="px-3 py-2">{props.labels.status}</th>
              </tr>
            </thead>
            <tbody>
              {props.songs.map((song) => (
                <tr
                  key={song.id}
                  className="border-b border-border last:border-0"
                >
                  <td className="px-3 py-3">
                    {song.audio_url ? (
                      <audio
                        className="h-9 w-72 max-w-full"
                        controls
                        preload="none"
                        src={song.audio_url}
                      />
                    ) : (
                      <span className="text-xs text-muted">
                        {song.audio_artifact_status}
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-3">
                    <p className="font-medium">{song.title}</p>
                    <p className="text-xs text-muted">{song.artist || "-"}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {formatSongOffset(song.start_ms)} -{" "}
                    {formatSongOffset(song.end_ms)}
                  </td>
                  <td className="px-3 py-3">{song.status}</td>
                </tr>
              ))}
              {props.songs.length === 0 ? (
                <tr>
                  <td className="px-3 py-8 text-center text-muted" colSpan={4}>
                    {props.labels.noRecognizedSongs}
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
