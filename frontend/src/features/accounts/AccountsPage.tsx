import { RefreshWarning } from "../../shared/ui/RefreshWarning";
import { UnsavedChangesGuard } from "../../shared/forms/UnsavedChangesGuard";
import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState, type FormEvent } from "react";
import {
  type AccountSortKey,
  type AccountEditForm,
  type AccountForm,
  type MeResponse,
  type AccountListResponse,
  type Account,
  type ManagerPolicy,
} from "../../shared/console/types";
import {
  emptyAccountEditForm,
  emptyAccountForm,
  defaultManagerPolicy,
  filterAccounts,
  accountUpdatePayload,
} from "./model";
import { requestJson } from "../../shared/api/client";
import { uiCopy } from "../../shared/console/copy";
import { accountToEditForm } from "../../shared/console/format";
import { AccountsPanel, AccountEditorDialog } from "./views";
import { useLanguage } from "../../app/preferences";

export default function AccountsPage() {
  const language = useLanguage();
  const queryClient = useQueryClient();
  const [editorDirty, setEditorDirty] = useState(false);
  const [accountSearch, setAccountSearch] = useState("");
  const [accountSort, setAccountSort] =
    useState<AccountSortKey>("username_asc");
  const [selectedAccountId, setSelectedAccountId] = useState<number | null>(
    null,
  );
  const [accountEditorOpen, setAccountEditorOpen] = useState(false);
  const [accountEditForm, setAccountEditForm] =
    useState<AccountEditForm>(emptyAccountEditForm);
  const [accountForm, setAccountForm] = useState<AccountForm>({
    ...emptyAccountForm,
    policy: { ...defaultManagerPolicy },
  });
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false,
  });
  const user = meQuery.data?.user;
  const canManageSystemSettings = user?.role === "SUPER_ADMIN";
  const ui = uiCopy[language];
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: () => requestJson<AccountListResponse>("/api/v1/accounts"),
    enabled: Boolean(canManageSystemSettings),
    retry: false,
    refetchInterval: 30000,
  });
  const accounts = useMemo(
    () => accountsQuery.data?.items ?? [],
    [accountsQuery.data?.items],
  );
  const accountTotal = accountsQuery.data?.total ?? accounts.length;
  const visibleAccounts = filterAccounts(accounts, accountSearch, accountSort);
  const selectedAccount = accounts.find(
    (account) => account.id === selectedAccountId,
  );
  const createAccountMutation = useMutation({
    mutationFn: () =>
      requestJson<Account>("/api/v1/accounts", {
        method: "POST",
        body: JSON.stringify({
          username: accountForm.username,
          password: accountForm.password,
          role: "MANAGER",
          enabled: accountForm.enabled,
          policy: accountForm.policy,
        }),
      }),
    onSuccess: () => {
      setAccountForm({
        ...emptyAccountForm,
        policy: { ...defaultManagerPolicy },
      });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
  const updateAccountMutation = useMutation({
    mutationFn: (request: {
      accountId: number;
      payload: ReturnType<typeof accountUpdatePayload> | { enabled: boolean };
    }) =>
      requestJson<Account>(`/api/v1/accounts/${request.accountId}`, {
        method: "PATCH",
        body: JSON.stringify(request.payload),
      }),
    onSuccess: () => {
      setAccountEditorOpen(false);
      setSelectedAccountId(null);
      setAccountEditForm(emptyAccountEditForm);
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
  const updatePolicyMutation = useMutation({
    mutationFn: (request: { accountId: number; policy: ManagerPolicy }) =>
      requestJson<ManagerPolicy>(
        `/api/v1/accounts/${request.accountId}/policy`,
        {
          method: "PUT",
          body: JSON.stringify(request.policy),
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
  const onAccountSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createAccountMutation.mutate();
  };
  const onAccountEditSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedAccountId) {
      return;
    }
    updateAccountMutation.mutate({
      accountId: selectedAccountId,
      payload: accountUpdatePayload(accountEditForm),
    });
  };
  if (!user) return null;
  if (accountsQuery.isError && !accountsQuery.data)
    return (
      <PageStatus
        retry={() => {
          void accountsQuery.refetch();
        }}
      />
    );
  if (accountsQuery.isLoading) return <PageStatus loading />;
  return (
    <div className="feature-page space-y-6">
      <RefreshWarning
        failed={[accountsQuery].some((query) => query.isError)}
        retry={() =>
          [accountsQuery]
            .filter((query) => query.isError)
            .forEach((query) => {
              void query.refetch();
            })
        }
      />
      <UnsavedChangesGuard
        dirty={
          (accountEditorOpen && editorDirty) ||
          JSON.stringify(accountForm) !== JSON.stringify(emptyAccountForm)
        }
      />
      <AccountsPanel
        accountForm={accountForm}
        accounts={visibleAccounts}
        createError={createAccountMutation.isError}
        createPending={createAccountMutation.isPending}
        currentUserId={user.id}
        labels={ui}
        policyPending={updatePolicyMutation.isPending}
        search={accountSearch}
        sort={accountSort}
        total={accountTotal}
        visibleTotal={visibleAccounts.length}
        updatePending={updateAccountMutation.isPending}
        onAccountFormChange={setAccountForm}
        onCreate={onAccountSubmit}
        onSearchChange={setAccountSearch}
        onSortChange={setAccountSort}
        onEdit={(account) => {
          setSelectedAccountId(account.id);
          setAccountEditForm(accountToEditForm(account));
          setEditorDirty(false);
          setAccountEditorOpen(true);
        }}
        onToggleEnabled={(account) =>
          updateAccountMutation.mutate({
            accountId: account.id,
            payload: { enabled: !account.enabled },
          })
        }
        onUpdatePolicy={(account, policy) =>
          updatePolicyMutation.mutate({ accountId: account.id, policy })
        }
      />

      {accountEditorOpen && selectedAccount ? (
        <AccountEditorDialog
          account={selectedAccount}
          currentUserId={user.id}
          form={accountEditForm}
          isSaving={updateAccountMutation.isPending}
          labels={ui}
          saveError={updateAccountMutation.isError}
          onCancel={() => {
            if (updateAccountMutation.isPending) return;
            if (
              editorDirty &&
              !window.confirm(
                language === "zh"
                  ? "放弃未保存的修改？"
                  : "Discard unsaved changes?",
              )
            )
              return;
            setEditorDirty(false);
            setAccountEditorOpen(false);
            setSelectedAccountId(null);
            setAccountEditForm(emptyAccountEditForm);
          }}
          onChange={(next) => {
            setEditorDirty(true);
            setAccountEditForm(next);
          }}
          onSubmit={onAccountEditSubmit}
        />
      ) : null}
    </div>
  );
}
