import { describe, expect, it } from "vitest";
import type { JobItem } from "./types";
import { jobActivityState } from "./jobActivity";

const now = Date.parse("2026-09-18T00:00:00Z");
const job: JobItem = {
  id: 1,
  type: "UPLOAD_BILIBILI",
  resource_class: "NETWORK",
  status: "RUNNING",
  priority: 10,
  attempts: 1,
  max_attempts: 3,
  run_after: "2026-09-17T20:00:00Z",
  created_at: "2026-09-17T20:00:00Z",
  updated_at: "2026-09-17T20:00:00Z",
};

describe("jobActivityState", () => {
  it("keeps a live job distinct from quiet external-tool progress", () => {
    expect(
      jobActivityState(
        {
          ...job,
          heartbeat_at: "2026-09-17T23:59:45Z",
          progress_updated_at: "2026-09-17T22:00:00Z",
        },
        now,
      ),
    ).toBe("active-without-progress");
  });

  it("warns only when the independent heartbeat is stale", () => {
    expect(
      jobActivityState(
        {
          ...job,
          heartbeat_at: "2026-09-17T23:58:00Z",
          progress_updated_at: "2026-09-17T23:59:50Z",
        },
        now,
      ),
    ).toBe("stale");
    expect(
      jobActivityState(
        {
          ...job,
          heartbeat_at: "2026-09-17 23:59:45",
          progress_updated_at: "2026-09-17 23:59:50",
        },
        now,
      ),
    ).toBe("active");
  });
});
