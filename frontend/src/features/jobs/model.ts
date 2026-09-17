import { type JobItem, type JobSortKey } from "../../shared/console/types";
import { includesSearch } from "../../shared/console/format";

export function filterJobs(
  items: JobItem[],
  search: string,
  sort: JobSortKey,
): JobItem[] {
  const filtered = items.filter((job) => {
    if (!search.trim()) {
      return true;
    }
    return (
      includesSearch(job.type, search) ||
      includesSearch(job.status, search) ||
      includesSearch(job.resource_class, search) ||
      includesSearch(job.business_key, search) ||
      includesSearch(job.profile_name, search) ||
      includesSearch(job.owner_username, search) ||
      includesSearch(job.last_error, search)
    );
  });
  return [...filtered].sort((left, right) => {
    if (sort === "run_after_asc") {
      return Date.parse(left.run_after) - Date.parse(right.run_after);
    }
    if (sort === "status_asc") {
      return (
        left.status.localeCompare(right.status) ||
        Date.parse(right.updated_at) - Date.parse(left.updated_at)
      );
    }
    return Date.parse(right.updated_at) - Date.parse(left.updated_at);
  });
}
