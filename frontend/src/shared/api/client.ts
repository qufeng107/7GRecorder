import type { ManagerPolicy, User } from "./contracts.generated";

export type Session = { user: User; policy?: ManagerPolicy };
export type ListResponse<T> = { items: T[] | null; total?: number };
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
export async function requestJson<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  if (!path.startsWith("/api/") || path.startsWith("//"))
    throw new Error("Only same-origin API paths are allowed");
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  const response = await fetch(path, {
    ...init,
    credentials: "include",
    headers,
  });
  if (!response.ok) {
    if (response.status === 401 && !["/api/v1/me", "/api/v1/auth/login"].includes(path)) window.dispatchEvent(new Event("7gr:session-expired"));
    const body = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      body.error?.code ?? "REQUEST_FAILED",
      body.error?.message ?? `Request failed (${response.status})`,
    );
  }
  return response.json() as Promise<T>;
}
