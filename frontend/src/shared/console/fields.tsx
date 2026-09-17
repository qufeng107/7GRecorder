import { type AdminCopy, type Credential, type JobItem } from "./types";
import { Search } from "lucide-react";
import { formatChinaDateParts, formatBytes } from "./format";
import { useLanguage } from "../../app/preferences";
import { jobActivityState } from "./jobActivity";

export function PermissionBadge(props: {
  enabled: boolean;
  label: string;
  labels: AdminCopy;
}) {
  return (
    <div className="rounded-md border border-border bg-white px-3 py-2">
      <p className="text-xs uppercase text-muted">{props.label}</p>
      <p
        className={`mt-1 text-sm font-semibold ${props.enabled ? "text-accent" : "text-muted"}`}
      >
        {props.enabled ? props.labels.allowed : props.labels.blocked}
      </p>
    </div>
  );
}

export function CredentialSelect(props: {
  credentials: Credential[];
  disabled?: boolean;
  label: string;
  labels: AdminCopy;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <select
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent disabled:bg-[#f3f4f1]"
        disabled={props.disabled}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      >
        <option value="">{props.labels.noCredentialSelected}</option>
        {props.credentials.map((credential) => (
          <option key={credential.id} value={credential.id}>
            {credential.account_label} ({credential.status})
          </option>
        ))}
      </select>
    </label>
  );
}

export function JSONTextArea(props: {
  disabled?: boolean;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <textarea
        aria-label={props.label}
        className="min-h-28 rounded-md border border-border bg-white px-3 py-2 font-mono text-xs font-normal outline-none focus:border-accent disabled:bg-[#f3f4f1]"
        disabled={props.disabled}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </label>
  );
}

export function Metric(props: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs uppercase text-muted">{props.label}</p>
      <p className="mt-1 text-sm font-semibold text-ink">{props.value}</p>
    </div>
  );
}

export function TableToolbar(props: {
  labels: AdminCopy;
  search: string;
  sort: string;
  sortOptions: Array<{ label: string; value: string }>;
  onSearchChange: (value: string) => void;
  onSortChange: (value: string) => void;
}) {
  return (
    <div className="mt-4 flex flex-col gap-3 rounded-md border border-border bg-white p-3 sm:flex-row sm:items-end sm:justify-between">
      <label className="flex min-w-0 flex-1 flex-col gap-1 text-sm font-medium">
        {props.labels.search}
        <div className="flex h-10 items-center gap-2 rounded-md border border-border bg-white px-3 focus-within:border-accent">
          <Search className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
          <input
            className="min-w-0 flex-1 text-sm font-normal outline-none"
            placeholder={props.labels.searchPlaceholder}
            type="search"
            value={props.search}
            onChange={(event) => props.onSearchChange(event.target.value)}
          />
        </div>
      </label>
      <label className="flex flex-col gap-1 text-sm font-medium sm:w-64">
        {props.labels.sortBy}
        <select
          className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
          value={props.sort}
          onChange={(event) => props.onSortChange(event.target.value)}
        >
          {props.sortOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}

export function TableTimeHeader(props: { labels: AdminCopy; title: string }) {
  return (
    <span className="block leading-4">
      <span className="block">{props.title}</span>
      <span className="block text-[11px] normal-case text-muted">
        {props.labels.chinaTime}
      </span>
    </span>
  );
}

export function TableDateTime(props: { value: string }) {
  const parts = formatChinaDateParts(props.value);
  return (
    <span className="block text-xs leading-5 text-muted">
      <span className="block whitespace-nowrap">{parts.date}</span>
      <span className="block whitespace-nowrap">{parts.time}</span>
    </span>
  );
}

export function TextField(props: {
  autoComplete?: string;
  disabled?: boolean;
  label: string;
  type?: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <input
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
        autoComplete={props.autoComplete}
        disabled={props.disabled}
        type={props.type ?? "text"}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </label>
  );
}

export function JobProgress(props: { job: JobItem }) {
  const language = useLanguage();
  const total = props.job.progress_total_bytes ?? 0;
  const current = props.job.progress_current_bytes ?? 0;
  const message = props.job.progress_message ?? "";
  const activity = jobActivityState(props.job);
  if (total <= 0 && !message && activity === "not-running") {
    return null;
  }
  const percent =
    total > 0
      ? Math.min(100, Math.max(0, Math.round((current / total) * 100)))
      : 0;
  return (
    <div className="mt-2 w-48">
      {total > 0 ? (
        <>
          <div className="h-1.5 overflow-hidden rounded-sm bg-[#e5e8e1]">
            <div
              className="h-full bg-accent"
              style={{ width: `${percent}%` }}
            />
          </div>
          <p className="mt-1 text-xs text-muted">
            {percent}% - {current > 0 ? formatBytes(current) : "0 B"} /{" "}
            {formatBytes(total)}
          </p>
        </>
      ) : null}
      {message ? (
        <p className="mt-1 break-words text-xs text-muted">{message}</p>
      ) : null}
      {props.job.status === "RUNNING" && props.job.heartbeat_at ? (
        <ActivityTime
          label={language === "zh" ? "任务心跳" : "Job heartbeat"}
          value={props.job.heartbeat_at}
        />
      ) : null}
      {props.job.status === "RUNNING" && props.job.progress_updated_at ? (
        <ActivityTime
          label={language === "zh" ? "进度更新" : "Progress updated"}
          value={props.job.progress_updated_at}
        />
      ) : null}
      {activity === "active-without-progress" ? (
        <p className="mt-1 text-xs text-accent" data-job-activity="active">
          {language === "zh"
            ? "任务仍在运行；当前阶段暂无可解析进度。"
            : "Job is active; this stage has no parseable progress."}
        </p>
      ) : null}
      {activity === "stale" ? (
        <p className="mt-1 text-xs text-amber-700" data-job-activity="stale">
          {language === "zh"
            ? "任务心跳超过 1 分钟未更新，请检查 Worker。"
            : "Job heartbeat is over one minute old; check the Worker."}
        </p>
      ) : null}
    </div>
  );
}

function ActivityTime({ label, value }: { label: string; value: string }) {
  const parts = formatChinaDateParts(value);
  return (
    <p className="mt-1 text-xs text-muted">
      {label}：{parts.date} {parts.time}
    </p>
  );
}

export function TextAreaField(props: {
  disabled?: boolean;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <textarea
        aria-label={props.label}
        className="min-h-28 rounded-md border border-border bg-white px-3 py-2 text-sm font-normal outline-none focus:border-accent disabled:bg-[#f3f4f1]"
        disabled={props.disabled}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </label>
  );
}

export function NumberField(props: {
  label: string;
  max?: number;
  min: number;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <input
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
        max={props.max}
        min={props.min}
        type="number"
        value={props.value}
        onChange={(event) => props.onChange(Number(event.target.value))}
      />
    </label>
  );
}

export function SelectField(props: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm font-medium">
      {props.label}
      <select
        className="h-10 rounded-md border border-border bg-white px-3 text-sm font-normal outline-none focus:border-accent"
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      >
        <option value="original">original</option>
        <option value="high">high</option>
        <option value="super">super</option>
        <option value="hd">hd</option>
        <option value="smooth">smooth</option>
        <option value="4k">4k</option>
        <option value="2k">2k</option>
        <option value="dolby">dolby</option>
        <option value="blue_ray_dolby">blue_ray_dolby</option>
      </select>
    </label>
  );
}

export function ToggleField(props: {
  disabled?: boolean;
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between gap-3 rounded-md border border-border bg-white px-3 py-2 text-sm font-medium">
      {props.label}
      <input
        className="h-4 w-4 accent-[#16867a]"
        checked={props.checked}
        disabled={props.disabled}
        type="checkbox"
        onChange={(event) => props.onChange(event.target.checked)}
      />
    </label>
  );
}
