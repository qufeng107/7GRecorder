import {
  type ManagerPolicy,
  type AccountForm,
  type AccountEditForm,
  type Account,
  type AccountSortKey,
} from "../../shared/console/types";
import { includesSearch } from "../../shared/console/format";

export const defaultManagerPolicy: ManagerPolicy = {
  updated_at: "",
  can_edit_recording_profile: true,
  can_edit_bilibili_module: true,
  can_edit_cos_module: true,
  can_edit_netease_module: true,
  can_manage_local_files: true,
};

export const emptyAccountForm: AccountForm = {
  username: "",
  password: "",
  enabled: true,
  policy: defaultManagerPolicy,
};

export const emptyAccountEditForm: AccountEditForm = {
  username: "",
  password: "",
  enabled: true,
};

export function accountUpdatePayload(form: AccountEditForm) {
  return {
    username: form.username,
    enabled: form.enabled,
    ...(form.password ? { password: form.password } : {}),
  };
}

export function filterAccounts(
  items: Account[],
  search: string,
  sort: AccountSortKey,
): Account[] {
  const filtered = items.filter((account) => {
    if (!search.trim()) {
      return true;
    }
    return (
      includesSearch(account.username, search) ||
      includesSearch(account.role, search)
    );
  });
  return [...filtered].sort((left, right) => {
    if (sort === "role_asc") {
      return (
        left.role.localeCompare(right.role) ||
        left.username.localeCompare(right.username, "zh-CN")
      );
    }
    return left.username.localeCompare(right.username, "zh-CN");
  });
}
