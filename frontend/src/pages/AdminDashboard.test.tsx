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
    expect(screen.getByText("Recorder Console")).toBeInTheDocument();
    expect(screen.getByText("Session")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
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

    fireEvent.click(await screen.findByRole("button", { name: /profiles/i }));
    expect(await screen.findByText("Recording Profiles")).toBeInTheDocument();
    expect(await screen.findByText("No profiles yet.")).toBeInTheDocument();
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

    fireEvent.click(await screen.findByRole("button", { name: /accounts/i }));
    expect(await screen.findByRole("heading", { name: "Accounts" })).toBeInTheDocument();
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

    fireEvent.click(await screen.findByRole("button", { name: /my account/i }));
    expect(await screen.findByRole("heading", { name: "My Account" })).toBeInTheDocument();
    expect(await screen.findAllByText("MANAGER")).toHaveLength(2);
    expect(await screen.findByText("Recording Profiles")).toBeInTheDocument();
    expect(await screen.findByText("Local Files")).toBeInTheDocument();
  });
});
