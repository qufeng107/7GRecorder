import type { JobItem } from "./types";

const HEARTBEAT_STALE_MS = 60_000;
const PROGRESS_QUIET_MS = 60_000;

function timestamp(value?: string): number {
  if (!value) return Number.NaN;
  const normalized = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(value)
    ? value
    : `${value.replace(" ", "T")}Z`;
  return Date.parse(normalized);
}

export type JobActivityState =
  | "not-running"
  | "active"
  | "active-without-progress"
  | "stale";

export function jobActivityState(
  job: JobItem,
  now = Date.now(),
): JobActivityState {
  if (job.status !== "RUNNING") return "not-running";
  const heartbeat = timestamp(job.heartbeat_at);
  if (!Number.isFinite(heartbeat) || now - heartbeat > HEARTBEAT_STALE_MS)
    return "stale";
  const progress = timestamp(job.progress_updated_at);
  if (!Number.isFinite(progress) || now - progress > PROGRESS_QUIET_MS)
    return "active-without-progress";
  return "active";
}
