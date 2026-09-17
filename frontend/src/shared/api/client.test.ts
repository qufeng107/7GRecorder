import { afterEach, expect, it, vi } from "vitest";
import { ApiError, requestJson } from "./client";
afterEach(() => vi.unstubAllGlobals());
it("rejects external API targets before fetch", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  await expect(requestJson("https://example.com/api/v1/jobs")).rejects.toThrow(
    "same-origin",
  );
  expect(fetch).not.toHaveBeenCalled();
});
it("preserves API error status and code", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({
            error: { code: "FORBIDDEN", message: "Not allowed" },
          }),
          { status: 403 },
        ),
      ),
  );
  await expect(requestJson("/api/v1/jobs")).rejects.toMatchObject({
    status: 403,
    code: "FORBIDDEN",
    message: "Not allowed",
  });
});
it("handles non-JSON failures without hiding their HTTP status", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response("upstream error", { status: 502 })),
  );
  const error = await requestJson("/api/v1/jobs").catch((e) => e);
  expect(error).toBeInstanceOf(ApiError);
  expect(error).toMatchObject({ status: 502 });
});
