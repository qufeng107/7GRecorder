import { Modal } from "../../shared/ui/Modal";
import {
  type ProfileForm,
  type AdminCopy,
  type Account,
  type RecordingProfile,
  type ProfileSortKey,
} from "../../shared/console/types";
import { type FormEvent, useState } from "react";
import { Archive, ArchiveRestore, Plus, Save, X } from "lucide-react";
import {
  TextField,
  SelectField,
  NumberField,
  ToggleField,
  TableToolbar,
} from "../../shared/console/fields";

export function ProfileEditorDialog(props: {
  archivePending: boolean;
  form: ProfileForm;
  isEditing: boolean;
  isSaving: boolean;
  labels: AdminCopy;
  ownerAccounts: Account[];
  profile?: RecordingProfile;
  restorePending: boolean;
  saveError: boolean;
  showOwner: boolean;
  onArchive: (profileId: number) => void;
  onCancel: () => void;
  onChange: (form: ProfileForm) => void;
  onRestore: (profileId: number) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const [confirmArchive, setConfirmArchive] = useState(false);
  const update = <K extends keyof ProfileForm>(
    key: K,
    value: ProfileForm[K],
  ) => {
    props.onChange({ ...props.form, [key]: value });
  };
  const isArchived = Boolean(props.profile?.archived_at);
  const ownerOptions = props.ownerAccounts.filter((account) => account.enabled);
  const selectedOwnerMissing =
    props.form.owner_user_id &&
    !ownerOptions.some(
      (account) => String(account.id) === props.form.owner_user_id,
    );

  return (
    <Modal
      title={
        props.isEditing ? props.labels.editProfile : props.labels.newProfile
      }
      onClose={props.onCancel}
    >
      <form
        className="w-full max-w-xl rounded-md border border-border bg-panel p-4 shadow-xl"
        onSubmit={props.onSubmit}
      >
        <fieldset
          className="min-w-0"
          disabled={
            props.isSaving || props.archivePending || props.restorePending
          }
        >
          <div className="flex items-center justify-between gap-3 border-b border-border pb-3">
            <div>
              <h2 className="text-base font-semibold">
                {props.isEditing
                  ? props.labels.editProfile
                  : props.labels.newProfile}
              </h2>
              {isArchived ? (
                <p className="mt-1 text-xs font-medium text-muted">
                  {props.labels.archivedProfile}
                </p>
              ) : null}
            </div>
            <button
              className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border text-ink hover:border-accent hover:text-accent"
              type="button"
              onClick={props.onCancel}
            >
              <X className="h-4 w-4" aria-hidden="true" />
              <span className="sr-only">{props.labels.close}</span>
            </button>
          </div>

          <div className="mt-4 grid gap-3">
            {props.showOwner ? (
              <label className="flex flex-col gap-1 text-sm font-medium">
                {props.labels.owner}
                <select
                  className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
                  value={props.form.owner_user_id}
                  onChange={(event) =>
                    update("owner_user_id", event.target.value)
                  }
                >
                  {selectedOwnerMissing ? (
                    <option value={props.form.owner_user_id}>
                      {props.labels.currentOwner}
                    </option>
                  ) : null}
                  {ownerOptions.map((account) => (
                    <option key={account.id} value={account.id}>
                      {account.username} ({account.role})
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            <TextField
              label={props.labels.name}
              value={props.form.name}
              onChange={(value) => update("name", value)}
            />
            <TextField
              label={props.labels.roomId}
              value={props.form.room_id}
              onChange={(value) => update("room_id", value)}
            />
            <TextField
              label={props.labels.streamer}
              value={props.form.streamer_name}
              onChange={(value) => update("streamer_name", value)}
            />
            <TextField
              label={props.labels.streamerUid}
              value={props.form.streamer_uid}
              onChange={(value) => update("streamer_uid", value)}
            />
            <TextField
              label={props.labels.timezone}
              value={props.form.timezone}
              onChange={(value) => update("timezone", value)}
            />
            <TextField
              label={props.labels.publicSlug}
              value={props.form.public_slug}
              onChange={(value) => update("public_slug", value)}
            />
            <SelectField
              label={props.labels.quality}
              value={props.form.quality}
              onChange={(value) => update("quality", value)}
            />
            <NumberField
              label={props.labels.segmentSeconds}
              min={60}
              value={props.form.segment_duration_sec}
              onChange={(value) => update("segment_duration_sec", value)}
            />
            <NumberField
              label={props.labels.finalizeGraceSeconds}
              min={0}
              value={props.form.finalize_grace_period_sec}
              onChange={(value) => update("finalize_grace_period_sec", value)}
            />
            <ToggleField
              label={props.labels.enabled}
              checked={props.form.enabled}
              onChange={(value) => update("enabled", value)}
            />
            <ToggleField
              label={props.labels.autoRecord}
              checked={props.form.auto_record}
              onChange={(value) => update("auto_record", value)}
            />
            <ToggleField
              label={props.labels.recordDanmaku}
              checked={props.form.record_danmaku}
              onChange={(value) => update("record_danmaku", value)}
            />
            <ToggleField
              label={props.labels.publicPage}
              checked={props.form.public_enabled}
              onChange={(value) => update("public_enabled", value)}
            />
          </div>

          {props.saveError ? (
            <p className="mt-3 text-sm text-red-700">
              {props.labels.profileSaveFailed}
            </p>
          ) : null}

          <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
            <div>
              {props.profile && isArchived ? (
                <button
                  className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-semibold text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                  disabled={props.restorePending}
                  type="button"
                  onClick={() => props.onRestore(props.profile!.id)}
                >
                  <ArchiveRestore className="h-4 w-4" aria-hidden="true" />
                  {props.labels.restoreProfile}
                </button>
              ) : null}
              {props.profile && !isArchived ? (
                <div className="flex flex-wrap items-center gap-2">
                  {confirmArchive ? (
                    <>
                      <button
                        className="inline-flex h-9 items-center justify-center rounded-md bg-red-700 px-3 text-sm font-semibold text-white disabled:opacity-60"
                        disabled={props.archivePending}
                        type="button"
                        onClick={() => props.onArchive(props.profile!.id)}
                      >
                        {props.labels.confirmArchive}
                      </button>
                      <button
                        className="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm font-medium text-ink"
                        type="button"
                        onClick={() => setConfirmArchive(false)}
                      >
                        {props.labels.cancel}
                      </button>
                    </>
                  ) : (
                    <button
                      className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-red-200 px-3 text-sm font-semibold text-red-700 hover:border-red-700 disabled:opacity-60"
                      disabled={props.archivePending}
                      type="button"
                      onClick={() => setConfirmArchive(true)}
                    >
                      <Archive className="h-4 w-4" aria-hidden="true" />
                      {props.labels.archiveProfile}
                    </button>
                  )}
                </div>
              ) : null}
            </div>

            <div className="flex items-center gap-2">
              <button
                className="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm font-medium text-ink"
                type="button"
                onClick={props.onCancel}
              >
                {props.labels.cancel}
              </button>
              <button
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
                disabled={props.isSaving}
                type="submit"
              >
                {props.isEditing ? (
                  <Save className="h-4 w-4" />
                ) : (
                  <Plus className="h-4 w-4" />
                )}
                {props.isEditing ? props.labels.save : props.labels.create}
              </button>
            </div>
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}

export function ProfileListPanel(props: {
  canEdit: boolean;
  labels: AdminCopy;
  profiles: RecordingProfile[];
  search: string;
  selectedProfileId: number | null;
  showOwner: boolean;
  sort: ProfileSortKey;
  total: number;
  visibleTotal: number;
  onCreate: () => void;
  onSearchChange: (value: string) => void;
  onSelect: (profile: RecordingProfile) => void;
  onSortChange: (value: ProfileSortKey) => void;
}) {
  return (
    <section className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">
          {props.labels.recordingProfiles}
        </h2>
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted">
            {props.labels.total(props.visibleTotal)} / {props.total}
          </span>
          {props.canEdit ? (
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white"
              type="button"
              onClick={props.onCreate}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
              {props.labels.new}
            </button>
          ) : null}
        </div>
      </div>
      <TableToolbar
        labels={props.labels}
        search={props.search}
        sort={props.sort}
        sortOptions={[
          { value: "name_asc", label: props.labels.sortName },
          { value: "room_asc", label: props.labels.sortRoom },
        ]}
        onSearchChange={props.onSearchChange}
        onSortChange={(value) => props.onSortChange(value as ProfileSortKey)}
      />
      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.name}</th>
              {props.showOwner ? (
                <th className="px-3 py-2 font-semibold">
                  {props.labels.ownerColumn}
                </th>
              ) : null}
              <th className="px-3 py-2 font-semibold">{props.labels.room}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.runtime}
              </th>
              <th className="px-3 py-2 font-semibold">{props.labels.sync}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.actions}
              </th>
            </tr>
          </thead>
          <tbody>
            {props.profiles.map((profile) => (
              <tr
                key={profile.id}
                className={
                  profile.id === props.selectedProfileId
                    ? "bg-[#f4f7f1]"
                    : "bg-white"
                }
              >
                <td className="px-3 py-3">
                  <button
                    className={`font-semibold ${props.canEdit ? "text-ink hover:text-accent" : "cursor-default text-ink"}`}
                    type="button"
                    onClick={() => {
                      if (props.canEdit) {
                        props.onSelect(profile);
                      }
                    }}
                  >
                    {profile.name}
                  </button>
                  <p className="mt-1 text-xs text-muted">
                    {profile.streamer_name}
                  </p>
                </td>
                {props.showOwner ? (
                  <td className="px-3 py-3 text-muted">
                    {profile.owner_username || profile.owner_user_id}
                  </td>
                ) : null}
                <td className="px-3 py-3 text-muted">{profile.room_id}</td>
                <td className="px-3 py-3 text-muted">
                  {profile.runtime.recorder_status}
                </td>
                <td className="px-3 py-3 text-muted">
                  {profile.runtime.sync_status}
                </td>
                <td className="px-3 py-3">
                  {!props.canEdit ? (
                    <span className="text-xs text-muted">
                      {props.labels.noAction}
                    </span>
                  ) : profile.archived_at ? (
                    <span className="rounded-md border border-border px-2 py-1 text-xs font-medium text-muted">
                      {props.labels.archived}
                    </span>
                  ) : (
                    <button
                      className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink hover:border-accent hover:text-accent"
                      type="button"
                      onClick={() => props.onSelect(profile)}
                    >
                      {props.labels.edit}
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {props.profiles.length === 0 ? (
              <tr>
                <td
                  className="px-3 py-8 text-center text-muted"
                  colSpan={props.showOwner ? 6 : 5}
                >
                  {props.profiles.length === 0 && props.search
                    ? props.labels.emptyFiltered
                    : props.labels.noProfiles}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );
}
