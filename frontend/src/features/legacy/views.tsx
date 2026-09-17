import {
  type AdminPage,
  type AdminCopy,
  type Language,
  type User,
} from "../../shared/console/types";
import {
  Activity,
  CloudUpload,
  FileVideo,
  LayoutDashboard,
  LogIn,
  LogOut,
  Music2,
  RefreshCw,
  Settings,
  UserCircle,
  Users,
} from "lucide-react";
import { type FormEvent } from "react";
import { TextField } from "../../shared/console/fields";

export function AdminNav(props: {
  activePage: AdminPage;
  canManageSystemSettings: boolean;
  canManageUploadSettings: boolean;
  labels: AdminCopy;
  onChange: (page: AdminPage) => void;
}) {
  const items: Array<{
    page: AdminPage;
    label: string;
    icon: typeof Activity;
  }> = [
    {
      page: "overview",
      label: props.labels.nav.overview,
      icon: LayoutDashboard,
    },
    { page: "profiles", label: props.labels.nav.profiles, icon: Activity },
    { page: "recordings", label: props.labels.nav.recordings, icon: FileVideo },
  ];

  if (props.canManageUploadSettings) {
    items.push({
      page: "uploads",
      label: props.labels.nav.uploads,
      icon: CloudUpload,
    });
  }

  if (props.canManageSystemSettings) {
    items.push({ page: "songs", label: props.labels.nav.songs, icon: Music2 });
  }

  items.push({ page: "jobs", label: props.labels.nav.jobs, icon: RefreshCw });

  if (props.canManageSystemSettings) {
    items.push({
      page: "accounts",
      label: props.labels.nav.accounts,
      icon: Users,
    });
    items.push({
      page: "system",
      label: props.labels.nav.system,
      icon: Settings,
    });
  }

  return (
    <nav
      className="mt-5 flex flex-wrap gap-2"
      aria-label={props.labels.navLabel}
    >
      {items.map(({ page, label, icon: Icon }) => {
        const isActive = props.activePage === page;
        return (
          <button
            key={page}
            className={`inline-flex h-10 items-center justify-center gap-2 rounded-md border px-3 text-sm font-medium shadow-sm ${
              isActive
                ? "border-accent bg-accent text-white"
                : "border-border bg-panel text-ink hover:border-accent hover:text-accent"
            }`}
            type="button"
            onClick={() => props.onChange(page)}
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
            {label}
          </button>
        );
      })}
    </nav>
  );
}

export function AccountMenu(props: {
  language: Language;
  labels: AdminCopy;
  logoutPending: boolean;
  user: User;
  onAccount: () => void;
  onLanguageChange: (language: Language) => void;
  onLogout: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-panel px-3 py-2 shadow-sm">
      <UserCircle className="h-5 w-5 text-accent" aria-hidden="true" />
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{props.user.username}</p>
        <p className="text-xs text-muted">{props.user.role}</p>
      </div>
      <LanguageControl
        language={props.language}
        labels={props.labels}
        onLanguageChange={props.onLanguageChange}
      />
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-medium text-ink hover:border-accent hover:text-accent"
        type="button"
        onClick={props.onAccount}
      >
        <UserCircle className="h-4 w-4" aria-hidden="true" />
        {props.labels.myAccount}
      </button>
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-ink px-3 text-sm font-medium text-white disabled:opacity-60"
        disabled={props.logoutPending}
        type="button"
        onClick={props.onLogout}
      >
        <LogOut className="h-4 w-4" aria-hidden="true" />
        {props.labels.signOut}
      </button>
    </div>
  );
}

export function LanguageControl(props: {
  language: Language;
  labels: AdminCopy;
  onLanguageChange: (language: Language) => void;
}) {
  return (
    <label className="flex items-center gap-2 rounded-md border border-border bg-panel px-3 py-2 text-xs font-medium text-muted shadow-sm">
      {props.labels.language}
      <select
        className="h-9 rounded-md border border-border bg-white px-2 text-sm font-medium text-ink outline-none focus:border-accent"
        value={props.language}
        onChange={(event) =>
          props.onLanguageChange(event.target.value as Language)
        }
      >
        <option value="zh">{props.labels.chinese}</option>
        <option value="en">{props.labels.english}</option>
      </select>
    </label>
  );
}

export function SessionPanel(props: {
  labels: AdminCopy;
  loginError: boolean;
  logoutPending: boolean;
  password: string;
  username: string;
  user?: User;
  onLoginSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onLogout: () => void;
  onPasswordChange: (value: string) => void;
  onUsernameChange: (value: string) => void;
}) {
  if (props.user) {
    return (
      <aside className="rounded-md border border-border bg-panel p-4 shadow-sm">
        <h2 className="text-sm font-semibold">{props.labels.session}</h2>
        <div className="mt-4 flex flex-col gap-4">
          <div>
            <p className="text-lg font-semibold">{props.user.username}</p>
            <p className="text-sm text-muted">{props.user.role}</p>
          </div>
          <button
            className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-ink px-3 text-sm font-medium text-white disabled:opacity-60"
            disabled={props.logoutPending}
            type="button"
            onClick={props.onLogout}
          >
            <LogOut className="h-4 w-4" aria-hidden="true" />
            {props.labels.signOut}
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className="rounded-md border border-border bg-panel p-4 shadow-sm">
      <h2 className="text-sm font-semibold">{props.labels.session}</h2>
      <form className="mt-4 flex flex-col gap-3" onSubmit={props.onLoginSubmit}>
        <TextField
          autoComplete="username"
          label={props.labels.username}
          value={props.username}
          onChange={props.onUsernameChange}
        />
        <TextField
          autoComplete="current-password"
          label={props.labels.password}
          type="password"
          value={props.password}
          onChange={props.onPasswordChange}
        />
        {props.loginError ? (
          <p className="text-sm text-red-700">{props.labels.loginFailed}</p>
        ) : null}
        <button className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white">
          <LogIn className="h-4 w-4" aria-hidden="true" />
          {props.labels.signIn}
        </button>
      </form>
    </aside>
  );
}
