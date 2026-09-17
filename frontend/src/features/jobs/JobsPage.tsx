import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../shared/api/client";
import {
  Activity,
  CheckCircle2,
  Clock3,
  RefreshCw,
  Search,
  TriangleAlert,
} from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { useLanguage } from "../../app/preferences";
import { Button } from "../../shared/ui/Button";
import { useJobAction, useJobs } from "./api";
import { copy } from "./copy";
import type { Job } from "../../shared/api/contracts.generated";

function date(value: string) {
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "—"
    : new Intl.DateTimeFormat("zh-CN", {
        timeZone: "Asia/Shanghai",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }).format(parsed);
}
export default function JobsPage() {
  const language = useLanguage(),
    t = copy[language];
  const query = useJobs(),
    action = useJobAction();
  const client = useQueryClient();
  useEffect(() => {
    if (query.error instanceof ApiError && query.error.status === 401)
      void client.invalidateQueries({ queryKey: ["me"] });
  }, [query.error, client]);
  const [params, setParams] = useSearchParams();
  const search = params.get("q") ?? "",
    sort = params.get("sort") ?? "updated_desc";
  function update(key: string, value: string) {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );
  }
  const all = query.data?.items ?? [];
  const jobs = all
    .filter((job) =>
      [
        job.type,
        job.status,
        job.resource_class,
        job.business_key,
        job.profile_name,
        job.owner_username,
        job.last_error,
      ].some((value) =>
        value?.toLowerCase().includes(search.trim().toLowerCase()),
      ),
    )
    .sort((a, b) =>
      sort === "run_after_asc"
        ? Date.parse(a.run_after) - Date.parse(b.run_after)
        : sort === "status_asc"
          ? a.status.localeCompare(b.status) ||
            Date.parse(b.updated_at) - Date.parse(a.updated_at)
          : Date.parse(b.updated_at) - Date.parse(a.updated_at),
    );
  function retry(job: Job) {
    const ambiguous =
      job.type === "UPLOAD_BILIBILI" && job.last_error_class === "AMBIGUOUS";
    if (ambiguous && !window.confirm(t.confirm)) return;
    action.mutate({ id: job.id, action: "retry", confirm: ambiguous });
  }
  const metrics = [
    { label: t.all, value: all.length, icon: Clock3 },
    {
      label: t.running,
      value: all.filter((j) => j.status === "RUNNING").length,
      icon: Activity,
    },
    {
      label: t.failed,
      value: all.filter((j) => j.status === "FAILED").length,
      icon: TriangleAlert,
    },
    {
      label: t.success,
      value: all.filter((j) => j.status === "SUCCEEDED").length,
      icon: CheckCircle2,
    },
  ];
  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="console-eyebrow">WORKSPACE / OPERATIONS</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            {t.title}
          </h1>
          <p className="mt-2 text-sm text-muted">{t.subtitle}</p>
        </div>
        <Button
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw
            size={16}
            className={query.isFetching ? "animate-spin" : ""}
          />
          {t.refresh}
        </Button>
      </header>
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        {metrics.map((m) => (
          <div className="console-card p-5" key={m.label}>
            <div className="flex justify-between text-sm text-muted">
              <span>{m.label}</span>
              <m.icon size={17} />
            </div>
            <p className="mt-4 text-3xl font-semibold tabular-nums">
              {query.isPending || query.isError ? "—" : m.value}
            </p>
          </div>
        ))}
      </div>
      <section className="console-card overflow-hidden" aria-label={t.title}>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
          <label className="relative flex min-w-0 flex-1 items-center gap-2">
            <Search size={17} className="absolute left-3 text-muted" />
            <input
              className="console-input w-full max-w-md pl-10"
              aria-label={t.search}
              placeholder={t.search}
              value={search}
              onChange={(e) => update("q", e.target.value)}
            />
          </label>
          <select
            className="console-input"
            aria-label={t.sort}
            value={sort}
            onChange={(e) => update("sort", e.target.value)}
          >
            <option value="updated_desc">{t.updated}</option>
            <option value="run_after_asc">{t.scheduled}</option>
            <option value="status_asc">{t.statusSort}</option>
          </select>
        </div>
        {query.isError ? (
          <div role="alert" className="p-8 text-center">
            <TriangleAlert className="mx-auto mb-3 text-amber-600" />
            <p>{t.failedLoad}</p>
            <Button
              className="mx-auto mt-4"
              onClick={() => void query.refetch()}
            >
              {t.retryLoad}
            </Button>
          </div>
        ) : query.isPending ? (
          <div role="status" className="p-16 text-center text-muted">
            {t.loading}
          </div>
        ) : jobs.length === 0 ? (
          <div className="p-16 text-center">
            <Clock3 className="mx-auto mb-4 text-muted" />
            <h2 className="font-medium">{search ? t.noMatch : t.empty}</h2>
            <p className="mt-2 text-sm text-muted">{t.emptyHelp}</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1000px] text-left text-sm">
              <thead className="console-table-head">
                <tr>
                  {[
                    t.job,
                    t.profile,
                    t.status,
                    t.attempts,
                    t.schedule,
                    t.error,
                    t.actions,
                  ].map((v) => (
                    <th className="px-5 py-3 font-medium" key={v}>
                      {v}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => (
                  <tr key={job.id} className="border-t border-border align-top">
                    <td className="px-5 py-5">
                      <p className="font-medium">
                        {t.types[job.type as keyof typeof t.types] ?? job.type}
                      </p>
                      <p className="mt-1 text-xs text-muted">
                        #{job.id} · {job.resource_class}
                      </p>
                      {job.business_key && (
                        <p className="mt-1 max-w-48 break-all text-xs text-muted">
                          {job.business_key}
                        </p>
                      )}
                    </td>
                    <td className="px-5 py-5">
                      {job.profile_name || "—"}
                      <p className="mt-1 text-xs text-muted">
                        {job.owner_username}
                      </p>
                    </td>
                    <td className="px-5 py-5">
                      <span className="console-badge" data-status={job.status}>
                        {t.states[job.status as keyof typeof t.states] ??
                          job.status}
                      </span>
                      <p className="mt-2 text-xs text-muted">
                        {date(job.updated_at)}
                      </p>
                      {job.progress_total_bytes &&
                      job.progress_total_bytes > 0 ? (
                        <progress
                          className="mt-2 h-1.5 w-28 accent-teal-600"
                          aria-label={t.progress}
                          max={job.progress_total_bytes}
                          value={job.progress_current_bytes ?? 0}
                        />
                      ) : null}
                      {job.progress_message && (
                        <p className="mt-1 max-w-40 text-xs text-muted">
                          {job.progress_message}
                        </p>
                      )}
                    </td>
                    <td className="px-5 py-5 tabular-nums">
                      {job.attempts} / {job.max_attempts}
                    </td>
                    <td className="whitespace-nowrap px-5 py-5 text-xs text-muted">
                      {date(job.run_after)}
                    </td>
                    <td className="max-w-60 break-words px-5 py-5 text-xs text-muted">
                      {job.last_error || job.last_error_class || "—"}
                    </td>
                    <td className="px-5 py-5">
                      <div className="flex gap-2">
                        {["FAILED", "CANCELLED"].includes(job.status) && (
                          <Button
                            disabled={action.isPending}
                            onClick={() => retry(job)}
                          >
                            {t.retry}
                          </Button>
                        )}
                        {["PENDING", "FAILED"].includes(job.status) && (
                          <Button
                            disabled={action.isPending}
                            onClick={() =>
                              action.mutate({ id: job.id, action: "cancel" })
                            }
                          >
                            {t.cancel}
                          </Button>
                        )}
                        {["RUNNING", "SUCCEEDED"].includes(job.status) && (
                          <span className="text-muted">{t.noAction}</span>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <footer className="border-t border-border px-5 py-3 text-xs text-muted">
          {t.limit}
        </footer>
      </section>
      {action.isError && (
        <p role="alert" className="text-red-600">
          {t.actionFailed}
        </p>
      )}
      {action.isSuccess && (
        <p role="status" className="text-sm text-muted">
          {t.actionDone}
        </p>
      )}
    </div>
  );
}
