import { createConsoleFixtures } from "./consoleFixtures";
import type { Plugin } from "vite";
import type { IncomingMessage } from "node:http";
import type { Job } from "../shared/api/contracts.generated";
async function body(req: IncomingMessage) {
  let text = "";
  for await (const chunk of req) {
    text += chunk;
    if (text.length > 8192) throw new Error("body too large");
  }
  return text;
}
export function mockPlugin(): Plugin {
  let scenario = "populated",
    signedIn = true;
  const stamp = "2026-09-17T10:20:00Z";
  function seed(): Job[] {
    return ["RUNNING", "FAILED", "PENDING", "SUCCEEDED", "FAILED"].map(
      (status, index) => ({
        id: index + 1,
        type: [
          "UPLOAD_COS_OBJECT",
          "UPLOAD_BILIBILI",
          "MERGE_UPLOAD_SOURCE",
          "UPLOAD_COS_OBJECT",
          "VERIFY_BILIBILI",
        ][index],
        resource_class: index === 2 ? "MEDIA" : "NETWORK",
        status,
        priority: 10,
        attempts: index === 2 ? 0 : 1,
        max_attempts: 3,
        run_after: stamp,
        created_at: stamp,
        updated_at: stamp,
        recording_profile_id: 1,
        profile_name: "晚风电台",
        owner_username: "demo",
        business_key: `demo:job:${index + 1}`,
        ...(index === 0
          ? {
              progress_current_bytes: 640000000,
              progress_total_bytes: 1000000000,
              progress_message: "上传中 · 64%",
            }
          : {}),
        ...(index === 1
          ? {
              last_error_class: "AMBIGUOUS",
              last_error: "投稿结果不确定，请核实创作中心后重试。",
            }
          : {}),
        ...(index === 4
          ? {
              last_error_class: "NETWORK",
              last_error: "连接超时，可稍后重试。",
            }
          : {}),
      }),
    );
  }
  let jobs = seed();
  let consoleFixtures = createConsoleFixtures();
  return {
    name: "local-synthetic-api",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const path = (req.url ?? "").split("?")[0];
        if (
          !path.startsWith("/api/") &&
          !path.startsWith("/health/") &&
          !path.startsWith("/__mock/")
        )
          return next();
        const json = (status: number, value: unknown) => {
          res.statusCode = status;
          res.setHeader("Content-Type", "application/json");
          res.end(JSON.stringify(value));
        };
        try {
          if (path === "/__mock/scenario" && req.method === "GET")
            return json(200, { scenario });
          if (path === "/__mock/scenario" && req.method === "POST") {
            const value = await body(req);
            if (
              ![
                "populated",
                "empty",
                "error",
                "loading",
                "manager",
                "forbidden",
                "signedout",
              ].includes(value)
            )
              return json(400, { error: { code: "INVALID_SCENARIO" } });
            scenario = value;
            consoleFixtures = createConsoleFixtures();
            signedIn = value !== "signedout";
            jobs = value === "empty" ? [] : seed();
            return json(200, { scenario });
          }
          const user = {
            id: 1,
            username: "demo",
            role: ["manager", "forbidden"].includes(scenario)
              ? "MANAGER"
              : "SUPER_ADMIN",
            enabled: true,
            created_at: stamp,
            updated_at: stamp,
          };
          if (path === "/api/v1/auth/login" && req.method === "POST") {
            await body(req);
            signedIn = true;
            return json(200, { user });
          }
          if (path === "/api/v1/auth/logout" && req.method === "POST") {
            signedIn = false;
            return json(200, { status: "ok" });
          }
          if (path.startsWith("/health/") || path === "/api/v1/system/health")
            return json(200, { status: "ok", release_sha: "local-fixtures" });
          if (!signedIn)
            return json(401, {
              error: { code: "NOT_AUTHENTICATED", message: "Login required" },
            });
          if (path === "/api/v1/me")
            return json(200, {
              user,
              policy: {
                can_edit_recording_profile: false,
                can_edit_bilibili_module: false,
                can_edit_cos_module: false,
                can_edit_netease_module: false,
                can_manage_local_files: false,
                updated_at: stamp,
              },
            });
          if (
            path === "/__mock/download" ||
            path === "/api/v1/upload-sources/20/outputs/200/download"
          ) {
            res.setHeader("Content-Type", "application/octet-stream");
            res.setHeader(
              "Content-Disposition",
              'attachment; filename="synthetic-review.txt"',
            );
            return res.end("Synthetic download fixture. No real recording.");
          }
          if (path.startsWith("/api/v1/") && scenario === "error")
            return json(503, { error: { code: "UNAVAILABLE" } });
          if (path.startsWith("/api/v1/") && scenario === "loading")
            await new Promise((resolve) => setTimeout(resolve, 3500));
          if (path.startsWith("/api/v1/jobs")) {
            if (scenario === "error" || scenario === "forbidden")
              return json(scenario === "error" ? 503 : 403, {
                error: {
                  code: scenario === "error" ? "UNAVAILABLE" : "FORBIDDEN",
                  message: "Synthetic failure",
                },
              });
            if (path === "/api/v1/jobs" && req.method === "GET")
              return json(200, { items: jobs, total: jobs.length });
            const match = path.match(
              /^\/api\/v1\/jobs\/(\d+)\/actions\/(retry|cancel)$/,
            );
            if (match && req.method === "POST") {
              const job = jobs.find((j) => j.id === Number(match[1]));
              if (!job) return json(404, { error: { code: "JOB_NOT_FOUND" } });
              const payload = JSON.parse((await body(req)) || "{}");
              if (
                match[2] === "retry" &&
                (!["FAILED", "CANCELLED"].includes(job.status) ||
                  (job.last_error_class === "AMBIGUOUS" &&
                    payload.confirm_ambiguous_bilibili !== true))
              )
                return json(400, { error: { code: "VALIDATION_FAILED" } });
              if (
                match[2] === "cancel" &&
                !["PENDING", "FAILED"].includes(job.status)
              )
                return json(400, { error: { code: "VALIDATION_FAILED" } });
              job.status = match[2] === "retry" ? "PENDING" : "CANCELLED";
              return json(200, job);
            }
          }
          const result = consoleFixtures(
            path,
            req.method ?? "GET",
            req.method === "GET" ? {} : JSON.parse((await body(req)) || "{}"),
            scenario,
            user.role,
          );
          if (result) return json(result.status, result.data);
          return json(501, {
            error: {
              code: "MOCK_NOT_IMPLEMENTED",
              message: "No local fixture for this endpoint",
            },
          });
        } catch {
          return json(400, { error: { code: "MOCK_BAD_REQUEST" } });
        }
      });
    },
  };
}
