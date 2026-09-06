import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AdminDashboard } from "./AdminDashboard";

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ error: { code: "NOT_AUTHENTICATED" } })
    })
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderWithClient() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false }
    }
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AdminDashboard />
    </QueryClientProvider>
  );
}

async function switchToEnglish() {
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "en" } });
  await screen.findByText("Recorder Console");
}

function mockSuperAdminFetch(extra?: (path: string) => Response | undefined) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request) => {
      const path = input instanceof Request ? input.url : input.toString();
      const custom = extra?.(path);
      if (custom) {
        return custom;
      }
      if (path.endsWith("/api/v1/me")) {
        return {
          ok: true,
          json: async () => ({
            user: { id: 1, username: "admin", role: "SUPER_ADMIN", enabled: true }
          })
        } as Response;
      }
      return {
        ok: true,
        json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
      } as Response;
    })
  );
}

describe("AdminDashboard", () => {
  it("renders the admin sign in shell", async () => {
    renderWithClient();
    await switchToEnglish();
    expect(screen.getByText("Recorder Console")).toBeInTheDocument();
    expect(screen.getByText("Session")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
  });

  it("renders an empty profile table when the API returns null items", async () => {
    mockSuperAdminFetch((path) => {
      if (path.endsWith("/api/v1/recording-profiles")) {
        return {
          ok: true,
          json: async () => ({ items: null, total: 0 })
        } as Response;
      }
      return undefined;
    });

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /profiles/i }));
    expect(await screen.findByText("No profiles yet.")).toBeInTheDocument();
  });

  it("renders accounts for super admins", async () => {
    mockSuperAdminFetch((path) => {
      if (path.endsWith("/api/v1/accounts")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              {
                id: 2,
                username: "manager",
                role: "MANAGER",
                enabled: true,
                profile_count: 1,
                policy: {
                  can_edit_recording_profile: true,
                  can_edit_bilibili_module: true,
                  can_edit_cos_module: true,
                  can_edit_netease_module: true,
                  can_manage_local_files: true
                }
              }
            ],
            total: 1
          })
        } as Response;
      }
      return undefined;
    });

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /accounts/i }));
    expect(await screen.findByRole("heading", { name: "Accounts" })).toBeInTheDocument();
    expect(await screen.findByText("manager")).toBeInTheDocument();
  });

  it("opens account editor for super admins", async () => {
    mockSuperAdminFetch((path) => {
      if (path.endsWith("/api/v1/accounts")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              {
                id: 2,
                username: "manager",
                role: "MANAGER",
                enabled: true,
                profile_count: 1,
                policy: {
                  can_edit_recording_profile: true,
                  can_edit_bilibili_module: true,
                  can_edit_cos_module: true,
                  can_edit_netease_module: true,
                  can_manage_local_files: true
                }
              }
            ],
            total: 1
          })
        } as Response;
      }
      return undefined;
    });

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /accounts/i }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    expect(await screen.findByRole("heading", { name: "Edit Account" })).toBeInTheDocument();
    expect(await screen.findByText("Leave blank to keep the current password.")).toBeInTheDocument();
  });

  it("renders the current manager account and policy", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const path = input instanceof Request ? input.url : input.toString();
        if (path.endsWith("/api/v1/me")) {
          return {
            ok: true,
            json: async () => ({
              user: { id: 2, username: "manager", role: "MANAGER", enabled: true },
              policy: {
                can_edit_recording_profile: true,
                can_edit_bilibili_module: false,
                can_edit_cos_module: false,
                can_edit_netease_module: false,
                can_manage_local_files: true
              }
            })
          } as Response;
        }
        return {
          ok: true,
          json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
        } as Response;
      })
    );

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /my account/i }));
    expect(await screen.findByRole("heading", { name: "My Account" })).toBeInTheDocument();
    expect(await screen.findAllByText("MANAGER")).toHaveLength(2);
    expect((await screen.findAllByText("Recording Profiles")).length).toBeGreaterThan(0);
    expect(await screen.findByText(/Local files/i)).toBeInTheDocument();
  });

  it("hides edit actions when a manager cannot edit profiles", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const path = input instanceof Request ? input.url : input.toString();
        if (path.endsWith("/api/v1/me")) {
          return {
            ok: true,
            json: async () => ({
              user: { id: 2, username: "manager", role: "MANAGER", enabled: true },
              policy: {
                can_edit_recording_profile: false,
                can_edit_bilibili_module: false,
                can_edit_cos_module: false,
                can_edit_netease_module: false,
                can_manage_local_files: false
              }
            })
          } as Response;
        }
        if (path.endsWith("/api/v1/recording-profiles")) {
          return {
            ok: true,
            json: async () => ({
              items: [
                {
                  id: 1,
                  name: "7G",
                  owner_user_id: 2,
                  owner_username: "manager",
                  platform: "bilibili",
                  room_id: "1741048619",
                  streamer_name: "streamer",
                  timezone: "Asia/Shanghai",
                  enabled: true,
                  public_enabled: false,
                  recording_settings: {
                    auto_record: true,
                    quality: "original",
                    record_danmaku: true,
                    segment_duration_sec: 1800,
                    finalize_grace_period_sec: 300
                  },
                  runtime: {
                    stream_status: "OFFLINE",
                    recorder_status: "IDLE",
                    sync_status: "SYNCED"
                  }
                }
              ],
              total: 1
            })
          } as Response;
        }
        return {
          ok: true,
          json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
        } as Response;
      })
    );

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /profiles/i }));
    expect(await screen.findByText("7G")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "New" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("renders recording start time as its own China-time column", async () => {
    mockSuperAdminFetch((path) => {
      if (path.endsWith("/api/v1/recordings")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              {
                id: 1,
                recording_profile_id: 1,
                profile_name: "7G",
                room_id: "1741048619",
                streamer_name: "streamer",
                title: "test recording",
                started_at: "2026-09-05T15:55:44Z",
                completed_at: "2026-09-05T15:57:15Z",
                duration_ms: 91000,
                recording_status: "COMPLETED",
                local_storage_status: "CLOSED",
                local_protected: false,
                files: [
                  {
                    id: 1,
                    recording_id: 1,
                    relative_path: "recordings/1741048619/test.flv",
                    original_name: "test.flv",
                    kind: "VIDEO",
                    file_status: "CLOSED",
                    size_bytes: 1024,
                    duration_ms: 91000,
                    closed_at: "2026-09-05T15:57:15Z"
                  }
                ]
              }
            ],
            total: 1
          })
        } as Response;
      }
      return undefined;
    });

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /recordings/i }));
    expect(await screen.findByText("Recording Time")).toBeInTheDocument();
    expect(await screen.findByText("test recording")).toBeInTheDocument();
    expect((await screen.findAllByText(/China Time/)).length).toBeGreaterThan(0);
  });

  it("opens recording details and marks short segments", async () => {
    mockSuperAdminFetch((path) => {
      if (path.endsWith("/api/v1/recordings")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              {
                id: 1,
                recording_profile_id: 1,
                profile_name: "7G",
                room_id: "1741048619",
                streamer_name: "streamer",
                title: "short clip",
                started_at: "2026-09-05T15:55:44Z",
                completed_at: "2026-09-05T15:57:15Z",
                duration_ms: 91000,
                recording_status: "COMPLETED",
                local_storage_status: "CLOSED",
                local_protected: false,
                files: [
                  {
                    id: 1,
                    recording_id: 1,
                    relative_path: "recordings/1741048619/short.flv",
                    original_name: "short.flv",
                    kind: "VIDEO",
                    file_status: "CLOSED",
                    size_bytes: 1024,
                    duration_ms: 91000,
                    closed_at: "2026-09-05T15:57:15Z"
                  },
                  {
                    id: 2,
                    recording_id: 1,
                    relative_path: "recordings/1741048619/short.xml",
                    original_name: "short.xml",
                    kind: "DANMAKU",
                    file_status: "CLOSED",
                    size_bytes: 512,
                    duration_ms: 0,
                    closed_at: "2026-09-05T15:57:15Z"
                  }
                ]
              }
            ],
            total: 1
          })
        } as Response;
      }
      return undefined;
    });

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /recordings/i }));
    expect(await screen.findByText("Short segment")).toBeInTheDocument();
    expect(await screen.findByText("Visible Size")).toBeInTheDocument();
    expect(await screen.findByText("Short Segments")).toBeInTheDocument();
    expect(await screen.findByText("Protected Recordings")).toBeInTheDocument();

    fireEvent.click(await screen.findByRole("button", { name: "Details" }));
    expect(await screen.findByRole("heading", { name: "Recording Details" })).toBeInTheDocument();
    expect((await screen.findAllByText("recordings/1741048619/short.flv")).length).toBeGreaterThan(1);
    expect(await screen.findByText("recordings/1741048619/short.xml")).toBeInTheDocument();
    expect((await screen.findAllByText("File Size")).length).toBeGreaterThan(1);
  });

  it("runs local cleanup from the system storage page after confirmation", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const path = input instanceof Request ? input.url : input.toString();
      if (path.endsWith("/api/v1/me")) {
        return {
          ok: true,
          json: async () => ({
            user: { id: 1, username: "admin", role: "SUPER_ADMIN", enabled: true }
          })
        } as Response;
      }
      if (path.endsWith("/api/v1/storage/local")) {
        return {
          ok: true,
          json: async () => ({
            data_root: "/data/7grecorder",
            disk_total_bytes: 100,
            disk_free_bytes: 10,
            disk_available_bytes: 10,
            indexed_video_bytes: 90,
            indexed_video_files: 1,
            protected_recordings: 0,
            completed_recordings: 1,
            settings_configured: true,
            health: "WARNING",
            need_reclaim_bytes: 40,
            target_video_bytes: 50,
            settings: {
              max_recording_bytes: 80,
              min_system_free_bytes: 20,
              cleanup_target_ratio: 0.5,
              absolute_emergency_free_bytes: 10
            }
          })
        } as Response;
      }
      if (path.includes("/api/v1/storage/local/cleanup-candidates")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              {
                recording_id: 1,
                profile_name: "7G",
                room_id: "1741048619",
                streamer_name: "streamer",
                title: "old recording",
                started_at: "2026-09-05T15:00:00Z",
                completed_at: "2026-09-05T16:00:00Z",
                duration_ms: 3600000,
                file_count: 1,
                reclaimable_bytes: 40
              }
            ],
            total: 1,
            preview_reclaimable_bytes: 40
          })
        } as Response;
      }
      if (path.endsWith("/api/v1/storage/local/actions/cleanup")) {
        return {
          ok: true,
          json: async () => ({
            deleted_recordings: 1,
            deleted_files: 1,
            reclaimed_bytes: 40,
            skipped_recordings: 0
          })
        } as Response;
      }
      return {
        ok: true,
        json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
      } as Response;
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(window, "confirm").mockReturnValue(true);

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: /system settings/i }));
    await screen.findByText("old recording");
    fireEvent.click(await screen.findByRole("button", { name: "Run Cleanup" }));

    expect(await screen.findByText(/Cleanup finished/)).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/storage/local/actions/cleanup",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("lists jobs and retries a failed job", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const path = input instanceof Request ? input.url : input.toString();
      if (path.endsWith("/api/v1/me")) {
        return {
          ok: true,
          json: async () => ({
            user: { id: 1, username: "admin", role: "SUPER_ADMIN", enabled: true }
          })
        } as Response;
      }
      if (path.includes("/api/v1/jobs?")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              {
                id: 1,
                recording_profile_id: 1,
                type: "SYNC_RECORDER_PROFILE",
                resource_class: "LIGHT",
                business_key: "profile:1:recorder:sync",
                status: "FAILED",
                priority: 0,
                attempts: 2,
                max_attempts: 3,
                run_after: "2026-09-06T08:00:00Z",
                last_error_class: "TRANSIENT",
                last_error: "temporary recorder sync failure",
                created_at: "2026-09-06T07:00:00Z",
                updated_at: "2026-09-06T08:01:00Z",
                profile_name: "7G",
                owner_username: "admin"
              }
            ],
            total: 1
          })
        } as Response;
      }
      if (path.endsWith("/api/v1/jobs/1/actions/retry")) {
        return {
          ok: true,
          json: async () => ({
            id: 1,
            type: "SYNC_RECORDER_PROFILE",
            resource_class: "LIGHT",
            status: "PENDING",
            priority: 0,
            attempts: 0,
            max_attempts: 3,
            run_after: "2026-09-06T08:02:00Z",
            created_at: "2026-09-06T07:00:00Z",
            updated_at: "2026-09-06T08:02:00Z"
          })
        } as Response;
      }
      return {
        ok: true,
        json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
      } as Response;
    });
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient();
    await switchToEnglish();

    fireEvent.click(await screen.findByRole("button", { name: "Jobs" }));
    expect(await screen.findByRole("heading", { name: "Jobs" })).toBeInTheDocument();
    expect(await screen.findByText("SYNC_RECORDER_PROFILE")).toBeInTheDocument();
    expect(await screen.findByText("temporary recorder sync failure")).toBeInTheDocument();

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/jobs/1/actions/retry",
        expect.objectContaining({ method: "POST" })
      );
    });
  });
});
