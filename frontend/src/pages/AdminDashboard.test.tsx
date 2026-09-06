import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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

describe("AdminDashboard", () => {
  it("renders the admin sign in shell", () => {
    renderWithClient();
    expect(screen.getByText("录播控制台")).toBeInTheDocument();
    expect(screen.getByText("会话")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /登录/i })).toBeInTheDocument();
  });

  it("renders an empty profile table when the API returns null items", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const path = input instanceof Request ? input.url : input.toString();
        if (path.endsWith("/api/v1/me")) {
          return {
            ok: true,
            json: async () => ({
              user: { id: 1, username: "admin", role: "SUPER_ADMIN", enabled: true }
            })
          } as Response;
        }
        if (path.endsWith("/api/v1/recording-profiles")) {
          return {
            ok: true,
            json: async () => ({ items: null, total: 0 })
          } as Response;
        }
        return {
          ok: true,
          json: async () => ({ status: "ok", release_sha: "test" })
        } as Response;
      })
    );

    renderWithClient();

    fireEvent.click(await screen.findByRole("button", { name: /录制配置/i }));
    expect(await screen.findByText("暂无录制配置。")).toBeInTheDocument();
  });

  it("renders accounts for super admins", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const path = input instanceof Request ? input.url : input.toString();
        if (path.endsWith("/api/v1/me")) {
          return {
            ok: true,
            json: async () => ({
              user: { id: 1, username: "admin", role: "SUPER_ADMIN", enabled: true }
            })
          } as Response;
        }
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
        return {
          ok: true,
          json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
        } as Response;
      })
    );

    renderWithClient();

    fireEvent.click(await screen.findByRole("button", { name: /账号管理/i }));
    expect(await screen.findByRole("heading", { name: "账号" })).toBeInTheDocument();
    expect(await screen.findByText("manager")).toBeInTheDocument();
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

    fireEvent.click(await screen.findByRole("button", { name: /我的账号/i }));
    expect(await screen.findByRole("heading", { name: "我的账号" })).toBeInTheDocument();
    expect(await screen.findAllByText("MANAGER")).toHaveLength(2);
    expect(await screen.findByText("录制配置")).toBeInTheDocument();
    expect(await screen.findByText("本地文件")).toBeInTheDocument();
  });

  it("renders recording start time as its own China-time column", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const path = input instanceof Request ? input.url : input.toString();
        if (path.endsWith("/api/v1/me")) {
          return {
            ok: true,
            json: async () => ({
              user: { id: 1, username: "admin", role: "SUPER_ADMIN", enabled: true }
            })
          } as Response;
        }
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
                  streamer_name: "七宫筱野",
                  title: "测试录像",
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
        return {
          ok: true,
          json: async () => ({ items: [], total: 0, status: "ok", release_sha: "test" })
        } as Response;
      })
    );

    renderWithClient();

    fireEvent.click(await screen.findByRole("button", { name: /录像文件/i }));
    expect(await screen.findByText("录制时间")).toBeInTheDocument();
    expect(await screen.findByText("测试录像")).toBeInTheDocument();
    expect((await screen.findAllByText(/中国时间/)).length).toBeGreaterThan(0);
  });
});
