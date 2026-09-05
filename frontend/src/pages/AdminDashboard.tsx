import { useState } from "react";
import type { FormEvent } from "react";
import { Activity, Archive, Database, HardDrive, LogIn, LogOut, ShieldCheck } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

type User = {
  id: number;
  username: string;
  role: "SUPER_ADMIN" | "MANAGER";
  enabled: boolean;
};

type MeResponse = {
  user: User;
};

type HealthResponse = {
  status: string;
  release_sha: string;
};

const statusRows = [
  { label: "Recording Core", value: "Waiting for profiles", icon: Activity },
  { label: "SQLite", value: "Migrated on deploy", icon: Database },
  { label: "Local Storage", value: "Always enabled", icon: HardDrive },
  { label: "Optional Modules", value: "Disabled until configured", icon: Archive },
  { label: "Deployment", value: "main-only production", icon: ShieldCheck }
];

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  const response = await fetch(path, {
    ...init,
    credentials: "include",
    headers
  });
  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
  }
  return (await response.json()) as T;
}

export function AdminDashboard() {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false
  });

  const healthQuery = useQuery({
    queryKey: ["system-health"],
    queryFn: () => requestJson<HealthResponse>("/api/v1/system/health"),
    retry: false,
    refetchInterval: 10000
  });

  const loginMutation = useMutation({
    mutationFn: () =>
      requestJson<MeResponse>("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password })
      }),
    onSuccess: () => {
      setPassword("");
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  });

  const logoutMutation = useMutation({
    mutationFn: () => requestJson<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  });

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    loginMutation.mutate();
  };

  const user = meQuery.data?.user;

  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <header className="flex flex-col gap-2 border-b border-border pb-5">
          <p className="text-sm font-medium text-accent">7GRecorder Admin</p>
          <h1 className="text-3xl font-semibold tracking-normal">Recorder Console</h1>
          <p className="max-w-3xl text-sm leading-6 text-muted">
            Production deploy is online. Sign in with the first SUPER_ADMIN account before creating
            recording profiles.
          </p>
        </header>

        <section className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
          <div className="grid gap-3 md:grid-cols-2">
            {statusRows.map(({ label, value, icon: Icon }) => (
              <article key={label} className="rounded-md border border-border bg-panel p-4 shadow-sm">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <h2 className="text-sm font-semibold">{label}</h2>
                    <p className="mt-2 text-sm leading-5 text-muted">{value}</p>
                  </div>
                  <Icon className="h-5 w-5 shrink-0 text-accent" aria-hidden="true" />
                </div>
              </article>
            ))}
          </div>

          <aside className="rounded-md border border-border bg-panel p-4 shadow-sm">
            <h2 className="text-sm font-semibold">Session</h2>
            {user ? (
              <div className="mt-4 flex flex-col gap-4">
                <div>
                  <p className="text-lg font-semibold">{user.username}</p>
                  <p className="text-sm text-muted">{user.role}</p>
                </div>
                <button
                  className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-ink px-3 text-sm font-medium text-white"
                  type="button"
                  onClick={() => logoutMutation.mutate()}
                >
                  <LogOut className="h-4 w-4" aria-hidden="true" />
                  Sign out
                </button>
              </div>
            ) : (
              <form className="mt-4 flex flex-col gap-3" onSubmit={onSubmit}>
                <label className="flex flex-col gap-1 text-sm font-medium">
                  Username
                  <input
                    className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
                    autoComplete="username"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                  />
                </label>
                <label className="flex flex-col gap-1 text-sm font-medium">
                  Password
                  <input
                    className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
                    autoComplete="current-password"
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                </label>
                {loginMutation.isError ? (
                  <p className="text-sm text-red-700">Login failed. Check the credentials.</p>
                ) : null}
                <button
                  className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-white disabled:opacity-60"
                  disabled={loginMutation.isPending}
                  type="submit"
                >
                  <LogIn className="h-4 w-4" aria-hidden="true" />
                  Sign in
                </button>
              </form>
            )}
          </aside>
        </section>

        <section className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-border pt-4 text-sm text-muted">
          <span>API: {healthQuery.data?.status ?? "checking"}</span>
          <span>Release: {healthQuery.data?.release_sha ?? "unknown"}</span>
        </section>
      </div>
    </main>
  );
}
