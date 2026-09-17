import { Modal } from "../../shared/ui/Modal";
import {
  type AdminCopy,
  type ManagerPolicy,
  type User,
  type AccountForm,
  type Account,
  type AccountSortKey,
  type PolicyFlag,
  type AccountEditForm,
} from "../../shared/console/types";
import {
  Metric,
  PermissionBadge,
  TextField,
  ToggleField,
  TableToolbar,
} from "../../shared/console/fields";
import { type FormEvent } from "react";
import { Save, UserPlus, X } from "lucide-react";

export function MyAccountPanel(props: {
  canManageSystemSettings: boolean;
  labels: AdminCopy;
  policy?: ManagerPolicy;
  user: User;
}) {
  return (
    <section className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
      <section className="min-w-0 rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.myAccount}</h2>
        <div className="mt-4 grid gap-3">
          <Metric label={props.labels.username} value={props.user.username} />
          <Metric label={props.labels.role} value={props.user.role} />
          <Metric
            label={props.labels.status}
            value={
              props.user.enabled ? props.labels.enabled : props.labels.disabled
            }
          />
        </div>
      </section>

      <section className="min-w-0 rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.access}</h2>
        {props.canManageSystemSettings ? (
          <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <Metric
              label={props.labels.systemSettings}
              value={props.labels.allowed}
            />
            <Metric
              label={props.labels.accounts}
              value={props.labels.allowed}
            />
            <Metric
              label={props.labels.profiles}
              value={props.labels.allOwners}
            />
          </div>
        ) : props.policy ? (
          <div className="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-3">
            <PermissionBadge
              labels={props.labels}
              label={props.labels.recordingProfiles}
              enabled={props.policy.can_edit_recording_profile}
            />
            <PermissionBadge
              labels={props.labels}
              label={props.labels.bilibiliConfig}
              enabled={props.policy.can_edit_bilibili_module}
            />
            <PermissionBadge
              labels={props.labels}
              label={props.labels.cosConfig}
              enabled={props.policy.can_edit_cos_module}
            />
            <PermissionBadge
              labels={props.labels}
              label={props.labels.neteaseConfig}
              enabled={props.policy.can_edit_netease_module}
            />
            <PermissionBadge
              labels={props.labels}
              label={props.labels.localFiles}
              enabled={props.policy.can_manage_local_files}
            />
          </div>
        ) : (
          <p className="mt-4 text-sm text-muted">
            {props.labels.policyUnavailable}
          </p>
        )}
      </section>
    </section>
  );
}

export function AccountsPanel(props: {
  accountForm: AccountForm;
  accounts: Account[];
  createError: boolean;
  createPending: boolean;
  currentUserId: number;
  labels: AdminCopy;
  policyPending: boolean;
  search: string;
  sort: AccountSortKey;
  total: number;
  visibleTotal: number;
  updatePending: boolean;
  onAccountFormChange: (form: AccountForm) => void;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
  onEdit: (account: Account) => void;
  onSearchChange: (value: string) => void;
  onSortChange: (value: AccountSortKey) => void;
  onToggleEnabled: (account: Account) => void;
  onUpdatePolicy: (account: Account, policy: ManagerPolicy) => void;
}) {
  const updateForm = <K extends keyof AccountForm>(
    key: K,
    value: AccountForm[K],
  ) => {
    props.onAccountFormChange({ ...props.accountForm, [key]: value });
  };
  const updateFormPolicy = (key: PolicyFlag, value: boolean) => {
    props.onAccountFormChange({
      ...props.accountForm,
      policy: { ...props.accountForm.policy, [key]: value },
    });
  };

  return (
    <section className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
      <form
        className="min-w-0 rounded-md border border-border bg-panel p-4 shadow-sm"
        onSubmit={props.onCreate}
      >
        <fieldset className="min-w-0" disabled={props.createPending}>
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold">{props.labels.newManager}</h2>
            <button
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
              disabled={props.createPending}
              type="submit"
            >
              <UserPlus className="h-4 w-4" aria-hidden="true" />
              {props.labels.create}
            </button>
          </div>
          <div className="mt-4 grid gap-3">
            <TextField
              autoComplete="username"
              label={props.labels.username}
              value={props.accountForm.username}
              onChange={(value) => updateForm("username", value)}
            />
            <TextField
              autoComplete="new-password"
              label={props.labels.initialPassword}
              type="password"
              value={props.accountForm.password}
              onChange={(value) => updateForm("password", value)}
            />
            <ToggleField
              label={props.labels.enabled}
              checked={props.accountForm.enabled}
              onChange={(value) => updateForm("enabled", value)}
            />
            <AccountPolicyFields
              labels={props.labels}
              policy={props.accountForm.policy}
              onChange={(key, value) => updateFormPolicy(key, value)}
            />
            {props.createError ? (
              <p className="text-sm text-red-700">
                {props.labels.accountCreationFailed}
              </p>
            ) : null}
          </div>
        </fieldset>
      </form>

      <section className="min-w-0 rounded-md border border-border bg-panel p-4 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold">{props.labels.accounts}</h2>
          <span className="text-sm text-muted">
            {props.labels.total(props.visibleTotal)} / {props.total}
          </span>
        </div>
        <TableToolbar
          labels={props.labels}
          search={props.search}
          sort={props.sort}
          sortOptions={[
            { value: "username_asc", label: props.labels.sortUsername },
            { value: "role_asc", label: props.labels.sortRole },
          ]}
          onSearchChange={props.onSearchChange}
          onSortChange={(value) => props.onSortChange(value as AccountSortKey)}
        />
        <div className="mt-4 overflow-x-auto rounded-md border border-border">
          <table className="w-full min-w-[640px] border-collapse text-left text-sm">
            <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
              <tr>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.account}
                </th>
                <th className="px-3 py-2 font-semibold">{props.labels.role}</th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.profiles}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.status}
                </th>
                <th className="px-3 py-2 font-semibold">
                  {props.labels.actions}
                </th>
              </tr>
            </thead>
            <tbody>
              {props.accounts.map((account) => (
                <tr key={account.id} className="bg-white align-top">
                  <td className="px-3 py-3">
                    <p className="font-semibold text-ink">{account.username}</p>
                    <p className="mt-1 text-xs text-muted">ID {account.id}</p>
                  </td>
                  <td className="px-3 py-3 text-muted">{account.role}</td>
                  <td className="px-3 py-3 text-muted">
                    {account.profile_count}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {account.enabled
                      ? props.labels.enabled
                      : props.labels.disabled}
                  </td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
                      <button
                        className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                        type="button"
                        onClick={() => props.onEdit(account)}
                      >
                        {props.labels.edit}
                      </button>
                      <button
                        className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                        disabled={
                          props.updatePending ||
                          account.id === props.currentUserId
                        }
                        type="button"
                        onClick={() => props.onToggleEnabled(account)}
                      >
                        {account.enabled
                          ? props.labels.disable
                          : props.labels.enable}
                      </button>
                      {account.policy ? (
                        <div className="grid gap-2 pt-1">
                          <AccountPolicyFields
                            compact
                            labels={props.labels}
                            policy={account.policy}
                            onChange={(key, value) =>
                              props.onUpdatePolicy(account, {
                                ...account.policy!,
                                [key]: value,
                              })
                            }
                            disabled={props.policyPending}
                          />
                        </div>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
              {props.accounts.length === 0 ? (
                <tr>
                  <td className="px-3 py-8 text-center text-muted" colSpan={5}>
                    {props.accounts.length === 0 && props.search
                      ? props.labels.emptyFiltered
                      : props.labels.noAccounts}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
    </section>
  );
}

export function AccountPolicyFields(props: {
  compact?: boolean;
  disabled?: boolean;
  labels: AdminCopy;
  policy: ManagerPolicy;
  onChange: (key: PolicyFlag, value: boolean) => void;
}) {
  const fields: Array<{ key: PolicyFlag; label: string }> = [
    { key: "can_edit_recording_profile", label: props.labels.editProfiles },
    { key: "can_edit_bilibili_module", label: props.labels.bilibiliConfig },
    { key: "can_edit_cos_module", label: props.labels.cosConfig },
    { key: "can_edit_netease_module", label: props.labels.neteaseConfig },
    { key: "can_manage_local_files", label: props.labels.localFiles },
  ];

  return (
    <div className={props.compact ? "grid gap-1" : "grid gap-2"}>
      {fields.map((field) => (
        <ToggleField
          key={field.key}
          checked={Boolean(props.policy[field.key])}
          disabled={props.disabled}
          label={field.label}
          onChange={(value) => props.onChange(field.key, value)}
        />
      ))}
    </div>
  );
}

export function AccountEditorDialog(props: {
  account: Account;
  currentUserId: number;
  form: AccountEditForm;
  isSaving: boolean;
  labels: AdminCopy;
  saveError: boolean;
  onCancel: () => void;
  onChange: (form: AccountEditForm) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const update = <K extends keyof AccountEditForm>(
    key: K,
    value: AccountEditForm[K],
  ) => {
    props.onChange({ ...props.form, [key]: value });
  };
  const isCurrentUser = props.account.id === props.currentUserId;

  return (
    <Modal title={props.labels.editAccount} onClose={props.onCancel}>
      <form
        className="w-full max-w-lg rounded-md border border-border bg-panel p-4 shadow-xl"
        onSubmit={props.onSubmit}
      >
        <fieldset className="min-w-0" disabled={props.isSaving}>
          <div className="flex items-center justify-between gap-3 border-b border-border pb-3">
            <div>
              <h2 className="text-base font-semibold">
                {props.labels.editAccount}
              </h2>
              <p className="mt-1 text-xs text-muted">
                ID {props.account.id} ·{" "}
                {isCurrentUser
                  ? props.labels.currentAccount
                  : props.account.role}
              </p>
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
            <TextField
              autoComplete="username"
              label={props.labels.username}
              value={props.form.username}
              onChange={(value) => update("username", value)}
            />
            <TextField
              autoComplete="new-password"
              label={props.labels.newPassword}
              type="password"
              value={props.form.password}
              onChange={(value) => update("password", value)}
            />
            <p className="-mt-2 text-xs text-muted">
              {props.labels.newPasswordHint}
            </p>
            <ToggleField
              disabled={isCurrentUser}
              label={props.labels.enabled}
              checked={props.form.enabled}
              onChange={(value) => update("enabled", value)}
            />
          </div>

          {props.saveError ? (
            <p className="mt-3 text-sm text-red-700">
              {props.labels.accountSaveFailed}
            </p>
          ) : null}

          <div className="mt-5 flex justify-end gap-2 border-t border-border pt-4">
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
              <Save className="h-4 w-4" aria-hidden="true" />
              {props.labels.saveAccount}
            </button>
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}
