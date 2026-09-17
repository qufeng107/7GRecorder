import {
  type JobItem,
  type AdminCopy,
  type JobSortKey,
} from "../../shared/console/types";
import { RefreshCw } from "lucide-react";
import { TableToolbar, JobProgress } from "../../shared/console/fields";
import {
  formatJobType,
  formatJobStatus,
  formatDateTime,
} from "../../shared/console/format";

export function JobsPanel(props: {
  cancelPending: boolean;
  isLoading: boolean;
  jobs: JobItem[];
  labels: AdminCopy;
  loadError: boolean;
  retryPending: boolean;
  search: string;
  sort: JobSortKey;
  total: number;
  visibleTotal: number;
  onCancel: (job: JobItem) => void;
  onRefresh: () => void;
  onRetry: (job: JobItem) => void;
  onSearchChange: (value: string) => void;
  onSortChange: (value: JobSortKey) => void;
}) {
  return (
    <section
      id="jobs"
      className="scroll-mt-6 rounded-md border border-border bg-panel p-4 shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{props.labels.jobs}</h2>
          <p className="mt-1 text-sm text-muted">
            {props.labels.total(props.visibleTotal)} / {props.total}
          </p>
        </div>
        <button
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-border px-3 text-sm font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
          disabled={props.isLoading}
          type="button"
          onClick={props.onRefresh}
        >
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
          {props.labels.refresh}
        </button>
      </div>

      {props.loadError ? (
        <p className="mt-3 text-sm text-red-700">{props.labels.jobsFailed}</p>
      ) : null}

      <TableToolbar
        labels={props.labels}
        search={props.search}
        sort={props.sort}
        sortOptions={[
          { value: "updated_desc", label: props.labels.sortUpdated },
          { value: "run_after_asc", label: props.labels.sortRunAfter },
          { value: "status_asc", label: props.labels.sortStatus },
        ]}
        onSearchChange={props.onSearchChange}
        onSortChange={(value) => props.onSortChange(value as JobSortKey)}
      />

      <div className="mt-4 overflow-hidden rounded-md border border-border">
        <table className="w-full border-collapse text-left text-sm">
          <thead className="bg-[#eef1eb] text-xs uppercase text-muted">
            <tr>
              <th className="px-3 py-2 font-semibold">{props.labels.job}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.profile}
              </th>
              <th className="px-3 py-2 font-semibold">{props.labels.status}</th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.attempts}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.runAfter}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.lastError}
              </th>
              <th className="px-3 py-2 font-semibold">
                {props.labels.actions}
              </th>
            </tr>
          </thead>
          <tbody>
            {props.jobs.map((job) => {
              const canRetry =
                job.status === "FAILED" || job.status === "CANCELLED";
              const canCancel = !["SUCCEEDED", "CANCELLED", "RUNNING"].includes(
                job.status,
              );
              return (
                <tr key={job.id} className="bg-white align-top">
                  <td className="px-3 py-3">
                    <p className="font-semibold text-ink">
                      {formatJobType(job.type, props.labels)}
                    </p>
                    <p className="mt-1 text-xs text-muted">
                      {job.resource_class}
                    </p>
                    {job.business_key ? (
                      <p className="mt-1 break-all text-xs text-muted">
                        {job.business_key}
                      </p>
                    ) : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>{job.profile_name || "-"}</p>
                    {job.owner_username ? (
                      <p className="mt-1 text-xs">{job.owner_username}</p>
                    ) : null}
                  </td>
                  <td className="px-3 py-3 text-muted">
                    <p>{formatJobStatus(job.status, props.labels)}</p>
                    <p className="mt-1 text-xs">
                      {formatDateTime(job.updated_at, props.labels)}
                    </p>
                    <JobProgress job={job} />
                  </td>
                  <td className="px-3 py-3 text-muted">
                    {job.attempts} / {job.max_attempts}
                  </td>
                  <td className="px-3 py-3 text-xs text-muted">
                    {formatDateTime(job.run_after, props.labels)}
                  </td>
                  <td className="max-w-md px-3 py-3 text-xs text-muted">
                    <span className="break-words">
                      {job.last_error || job.last_error_class || "-"}
                    </span>
                  </td>
                  <td className="px-3 py-3">
                    <div className="flex flex-col items-start gap-2">
                      {canRetry ? (
                        <button
                          className="inline-flex h-8 items-center justify-center gap-2 rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                          disabled={props.retryPending}
                          type="button"
                          onClick={() => props.onRetry(job)}
                        >
                          <RefreshCw
                            className="h-3.5 w-3.5"
                            aria-hidden="true"
                          />
                          {props.labels.retry}
                        </button>
                      ) : null}
                      {canCancel ? (
                        <button
                          className="inline-flex h-8 items-center justify-center rounded-md border border-border px-3 text-xs font-medium text-ink hover:border-accent hover:text-accent disabled:opacity-60"
                          disabled={props.cancelPending}
                          type="button"
                          onClick={() => props.onCancel(job)}
                        >
                          {props.labels.cancel}
                        </button>
                      ) : null}
                      {!canRetry && !canCancel ? (
                        <span className="text-xs text-muted">
                          {props.labels.noAction}
                        </span>
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
            {props.jobs.length === 0 ? (
              <tr>
                <td className="px-3 py-8 text-center text-muted" colSpan={7}>
                  {props.isLoading
                    ? props.labels.checking
                    : props.jobs.length === 0 && props.search
                      ? props.labels.emptyFiltered
                      : props.labels.noJobs}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );
}
