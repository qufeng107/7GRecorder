# 7GRecorder — API 设计约定

## 1. 定位

7GRecorder 前后端分离，但生产环境保持同域。

API 只承担：

- 管理后台业务操作；
- 公开主播页面读取公开数据；
- BililiveRecorder 等内部组件回调/控制边界。

API 不承担媒体文件在内部服务之间的搬运。大文件始终通过文件系统、COS 或外部平台传输。

---

## 2. API 分区

### Admin API

```text
/api/v1/*
```

要求登录、Ownership/Policy 校验和 CSRF 防护。

MANAGER 的 owner/profile 范围必须从登录 Session 推导；普通 Manager Request 不允许通过提交 `owner_user_id` 来选择其他账号资源。SUPER_ADMIN 跨账号操作使用明确的后台权限路径/校验。

### Public API

```text
/api/public/v1/*
```

无需登录，只返回明确允许公开的数据。

### Internal API

```text
/internal/v1/*
```

只供 BililiveRecorder/容器内组件调用。

生产 Nginx 默认**不转发公网 `/internal/*`**；BililiveRecorder 直接通过 Docker 内部网络访问 GoFrame 服务。

生产 Frontend/API 同域，默认不开放宽泛 CORS。跨域公开 API 只有出现真实需求后再单独设计。

---

## 3. 基本协议

- REST JSON；
- UTF-8；
- HTTP 状态码表达成功/失败；
- API Version 固定在 URL；
- ID 使用 SQLite INTEGER，对外表现为 JSON integer；
- 时间统一使用 UTC RFC3339；
- 状态枚举使用文档定义的大写值；
- 空值使用 JSON `null`，不使用空字符串表示“未知”。

成功对象直接返回资源 DTO，不再额外包一层通用：

```json
{"code": 0, "data": ...}
```

避免无收益协议包装。

列表统一：

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 0
}
```

第一版只使用 page/page_size，不做 cursor pagination。

---

## 4. 错误格式

所有 Admin/Public API 错误统一：

```json
{
  "error": {
    "code": "RECORDING_ACTIVE",
    "message": "Recording profile cannot change room while a recording is active.",
    "details": null,
    "request_id": "..."
  }
}
```

规则：

- `code`：稳定、面向前端判断的机器码；
- `message`：可读描述；
- `details`：字段验证等可选结构；
- `request_id`：用于日志排查；
- 不把内部 stack trace、SQL、Cookie、Token、绝对文件路径返回给客户端。

典型 HTTP 状态：

```text
400 validation / bad request
401 not logged in
403 policy / ownership denied
404 resource not found or not visible
409 business state conflict
422 semantically invalid action
429 optional future rate protection
500 unexpected internal error
503 dependent component temporarily unavailable
```

---

## 5. Admin API 第一版资源

### Auth

```text
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/me
```

`GET /api/v1/me` returns the current session user. For MANAGER accounts it also returns the account's `policy` so the UI can render account-level capabilities without granting access to global account management.

### Accounts / Policy

SUPER_ADMIN：

```text
GET    /api/v1/accounts
POST   /api/v1/accounts
GET    /api/v1/accounts/{id}
PATCH  /api/v1/accounts/{id}
PUT    /api/v1/accounts/{id}/policy
```

不物理删除有历史数据的账号，使用 enabled/archived 语义。

Early production bootstrap implements SUPER_ADMIN-only account management for Manager accounts:

- `GET /api/v1/accounts` returns users with profile counts and ManagerPolicy values.
- `POST /api/v1/accounts` creates MANAGER accounts with an initial password and policy.
- `GET /api/v1/accounts/{id}` returns one account.
- `PATCH /api/v1/accounts/{id}` updates username, password, or enabled status; the current SUPER_ADMIN cannot disable itself.
- `PUT /api/v1/accounts/{id}/policy` updates ManagerPolicy for MANAGER accounts only.

The first UI intentionally does not create additional SUPER_ADMIN accounts.

### Recording Profiles

```text
GET    /api/v1/recording-profiles
POST   /api/v1/recording-profiles
GET    /api/v1/recording-profiles/{id}
PATCH  /api/v1/recording-profiles/{id}
GET    /api/v1/recording-profiles/{id}/runtime
PUT    /api/v1/recording-profiles/{id}/recording-settings
```

Profile 保存规则：

```text
validate
→ SQLite 保存 Desired State
→ sync_status=PENDING
→ enqueue idempotent SYNC_RECORDER_PROFILE
→ response
```

Recorder 暂时不可用不能阻止管理网站保存配置。

正在 ACTIVE/FINALIZING Recording 时，第一版禁止修改：

- room_id；
- quality；
- segment_duration；
- record_danmaku 等会改变底层录制行为的核心参数。

`archived=true` 表示可恢复的软归档：Profile 会被停用、清除 active room 占用并触发 Recorder 同步停止该 room 的 desired recording；历史 Recording/File metadata 与本地录像不删除。恢复时使用 `archived=false`，需要继续录制时同时保存 `enabled=true`。

### Credentials / Module Config

```text
GET    /api/v1/credentials
POST   /api/v1/credentials
GET    /api/v1/credentials/{id}
PATCH  /api/v1/credentials/{id}
PUT    /api/v1/credentials/{id}/secret
POST   /api/v1/credentials/{id}/actions/verify
```

Secret 只写不读。

模块配置：

```text
GET/PUT /api/v1/recording-profiles/{id}/publishing/bilibili
GET/PUT /api/v1/recording-profiles/{id}/storage/cos
GET/PUT /api/v1/recording-profiles/{id}/songs/settings
```

未配置或 `enabled=false` 时，对应 Reconciler 不创建 Job。

Early upload-module bootstrap implements credential metadata creation and module configuration:

- `GET /api/v1/credentials` lists credential metadata only; encrypted secrets are never returned.
- `POST /api/v1/credentials` creates a USER or SYSTEM credential and stores the submitted secret encrypted with the
  server master key.
- `GET/PUT /api/v1/recording-profiles/{id}/publishing/bilibili` reads or saves the Bilibili publishing profile.
- `GET/PUT /api/v1/recording-profiles/{id}/storage/cos` reads or saves the COS storage profile.
- `POST /api/v1/upload-modules/actions/reconcile` is SUPER_ADMIN-only and creates idempotent Bilibili/COS upload jobs
  for `READY_TO_UPLOAD` upload sources. The first bootstrap creates jobs only; external upload execution is added after
  pinned biliup/COS fixtures are captured.

### Recordings

```text
GET /api/v1/recordings
GET /api/v1/recordings/{id}
GET /api/v1/recordings/{id}/sessions
GET /api/v1/recordings/{id}/files
GET /api/v1/recordings/{id}/publications
GET /api/v1/recordings/{id}/cos-objects
GET /api/v1/recordings/{id}/songs
```

动作：

```text
POST /api/v1/recordings/{id}/actions/protect-local
POST /api/v1/recordings/{id}/actions/unprotect-local
POST /api/v1/recordings/{id}/actions/delete-local
```

Early production bootstrap implements `protect-local` and `unprotect-local` only. They toggle
`recordings.local_protected` and must not delete or move files. `delete-local` remains reserved for the later
rolling cleanup workflow.

Early production bootstrap implements system-level manual local cleanup as:

```text
POST /api/v1/storage/local/actions/cleanup
```

The endpoint is SUPER_ADMIN-only. It executes the same oldest completed, unprotected local cleanup policy shown by
`cleanup-candidates`, but only when the current storage policy reports bytes need to be reclaimed. Each run is bounded
by `max_recordings` and must re-check that the recording is completed, unprotected, not already locally deleted, has no
WRITING files, and is not referenced by RUNNING jobs before deleting local video files. Metadata is retained and marked
`DELETED`.

Early production bootstrap also exposes:

```text
POST /api/v1/recording-files/reconcile
```

This endpoint indexes existing local video files under `DATA_ROOT/recordings` into SQLite. It must not delete,
move, upload, or otherwise mutate recording files.

前端不得直接 PATCH `recording_status`、`local_storage_status` 等内部状态。

Recording group diagnostics:

```text
GET /api/v1/recording-groups?max_gap_seconds=600&short_threshold_seconds=180
```

This endpoint computes read-only groups from visible completed local recordings. Adjacent recordings from the same
profile are grouped when the gap from previous completion to next start is less than or equal to the configured
threshold. It returns source recordings, group time window, segment/file counts, total size, total duration, max gap,
short-segment signal, and whether the group is eligible for a later merge job. It must not merge, move, upload, delete,
or rewrite files.

Upload sources are the durable upload-facing recording list:

```text
GET  /api/v1/upload-sources?merge_gap_seconds=600
POST /api/v1/upload-sources/actions/discover?merge_gap_seconds=600
GET  /api/v1/upload-sources/{id}/download
```

Discovery is SUPER_ADMIN-only and idempotent. It creates upload sources only when a profile is not currently live or
recording and the newest segment in a continuous group has been completed for longer than the merge gap threshold. The
same threshold controls both grouping adjacent segments and waiting before finalizing a group. Optional upload modules
must process only upload sources whose status is `READY_TO_UPLOAD`. Multi-segment sources become `READY_TO_UPLOAD`
after the `MERGE_UPLOAD_SOURCE` job writes a derived file under `DATA_ROOT/upload-sources`. Upload source download uses
the same authenticated `X-Accel-Redirect` pattern as recording file download.

### Files / Download

```text
GET /api/v1/recording-files/{id}
GET /api/v1/recording-files/{id}/download?source=local
GET /api/v1/recording-files/{id}/download?source=cos
```

Early production bootstrap implements local file download as:

```text
GET /api/v1/recording-files/{id}/download
```

This first version only serves local, closed video files. The backend must authenticate the session, check
profile visibility, resolve the DB `relative_path` under `DATA_ROOT`, and return `X-Accel-Redirect` for the
Nginx internal media location. COS downloads remain a later module.

下载行为见第 9 节。

### Publications

```text
GET /api/v1/publications
GET /api/v1/publications/{id}
POST /api/v1/publications/{id}/actions/retry
POST /api/v1/publications/{id}/actions/verify
```

### Songs

```text
GET    /api/v1/songs
POST   /api/v1/songs
GET    /api/v1/songs/{id}
PATCH  /api/v1/songs/{id}

POST /api/v1/songs/{id}/actions/cut
POST /api/v1/songs/{id}/actions/confirm
POST /api/v1/songs/{id}/actions/reject
```

### Jobs

```text
GET /api/v1/jobs
GET /api/v1/jobs/{id}
POST /api/v1/jobs/{id}/actions/retry
POST /api/v1/jobs/{id}/actions/cancel
```

Current admin API supports operational visibility for queued work. Super admins can list and operate on all jobs.
Managers can list and operate only on jobs attached to their own recording profiles. `retry` is allowed only for
`FAILED` and `CANCELLED` jobs and resets attempts, locks, and last error fields. `cancel` is allowed for queued or
failed non-terminal jobs, but not for `RUNNING`, `SUCCEEDED`, or already `CANCELLED` jobs.

### Storage / System

```text
GET /api/v1/storage/local
GET /api/v1/storage/local/cleanup-candidates
PUT /api/v1/storage/local/settings   # SUPER_ADMIN only

GET /api/v1/system/health
GET /api/v1/system/version
```

Early production bootstrap implements `GET /api/v1/storage/local` for SUPER_ADMIN only. It returns data-root disk
capacity, available bytes, indexed local video file count/bytes, completed recording count, and protected
recording count, the effective storage policy, health status, target video bytes, and needed reclaim bytes.

`PUT /api/v1/storage/local/settings` remains the canonical design path. Early production bootstrap accepts the
same payload on `PUT /api/v1/storage/local` to keep the temporary admin UI simple:

```json
{
  "max_recording_bytes": 75161927680,
  "min_system_free_bytes": 10737418240,
  "cleanup_target_ratio": 0.85,
  "absolute_emergency_free_bytes": 5368709120
}
```

All values must be positive, `cleanup_target_ratio` must be greater than 0 and less than 1, and emergency free
bytes must not exceed the normal minimum free bytes. Saving settings must not delete or move files.

`GET /api/v1/storage/local/cleanup-candidates` is a dry-run preview only. It returns the oldest unprotected
completed recordings with closed local video files and estimated reclaimable bytes. It must not delete, move, or
mark any files.

---

## 6. Public API

第一版只预留最小边界：

```text
GET /api/public/v1/streamers/{slug}
GET /api/public/v1/streamers/{slug}/recordings
GET /api/public/v1/streamers/{slug}/songs
```

只返回：

- `public_enabled=true` Profile；
- 已确认 Song；
- 已 VERIFIED 且允许公开的 Publication。

Public DTO 必须独立定义，禁止复用 Admin DTO。

Public API 永远不返回：

- 本地文件路径；
- COS object key 内部细节；
- Credential ID/状态；
- Job/Error 内部信息；
- Manager/User 信息；
- Recorder Session 原始 payload。

---

## 7. Internal API

Recorder Webhook：

```text
POST /internal/v1/recorder/webhook
```

要求：

1. 快速解析；
2. `event_id` 幂等写入；
3. 在 SQLite transaction 内更新必要业务状态；
4. 需要的后续工作只 enqueue Job；
5. 尽快返回 2xx。

Webhook Handler 不执行 FFmpeg、上传、AI 等重任务。

Internal API 不通过公网 Nginx 路由暴露。

---

## 8. Action Endpoint 原则

对于会改变业务状态、有副作用或需要额外校验的操作，使用：

```text
POST /resource/{id}/actions/{action}
```

而不是让前端：

```text
PATCH status=...
```

例如：

- retry；
- cancel；
- protect；
- delete；
- verify；
- confirm。

业务状态只由 Application Service/Job Handler 改变。

---

## 9. 大文件下载

Go 应用不直接复制数 GB 视频字节。

### Local

```text
Browser
→ authenticated Admin API
→ ownership/file state check
→ X-Accel-Redirect
→ Nginx internal location
→ file
```

要求：

- Nginx `internal` location；
- DB 只保存受控相对路径；
- Backend 解析后必须确认目标仍位于配置的数据根目录内；
- 支持 HTTP Range；
- 不把真实服务器绝对路径暴露给浏览器。

### COS

```text
Admin API
→ ownership/status check
→ Tencent COS short-lived signed URL
→ browser downloads from COS
```

默认签名只短时间有效。

### Bilibili

返回外部稿件 URL，不把 Bilibili 当原文件下载代理。

Songs 的本地音频也沿用受鉴权的 Nginx internal/Range 方案；公开 Song 后续可使用独立 Public media endpoint。

第一版不提供“把整场多分段 Recording 临时打 ZIP 再下载”：

- 会额外占用大量本地磁盘；
- 会产生高 IO/CPU；
- 与滚动存储目标冲突。

管理后台按 Recording 展示并下载各个原始分段；如果 COS 存在，对应分段可从 COS 下载。

---

## 10. Polling

第一版不建立 WebSocket 基础设施。

推荐：

```text
Dashboard / Runtime       5s
Active Recording          3–5s
Jobs                      5s
Storage status            10–30s
History                   user refresh / query invalidation
```

需要更实时能力时优先评估 SSE。

---

## 11. OpenAPI 与前端契约

GoFrame Backend 是 API Contract 来源。

开发流程：

```text
Go Request/Response DTO
→ OpenAPI
→ TypeScript generated types/client（或 typed wrapper）
→ React
```

要求：

- Frontend 不手写与后端重复的 enum/DTO；
- API Contract 变化必须同时通过 Backend 和 Frontend typecheck；
- Generated code 不是产品规格，产品规格仍以 docs 为准。

---

## 12. 幂等与并发请求

- Webhook：`event_id` 幂等；
- Job：`business_key` 幂等；
- Action API：Application 层检查当前业务状态；
- 第一版不引入通用 `Idempotency-Key` 基础设施。

重复点击 retry/protect 等动作必须得到稳定结果，不产生重复 Publication/Job。
