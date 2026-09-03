# 7GRecorder — 技术架构

## 1. 架构定位

7GRecorder 采用：

> **GoFrame 轻量模块化单体 Backend + React SPA + SQLite 持久任务系统 + 外部录制/媒体工具**

设计目标：

- 精简实用；
- 单机运行；
- 低磁盘/内存占用；
- 少中间件；
- 多账号、多直播间；
- 可恢复、可排错；
- 易 AI Coding；
- 可选功能之间互不形成硬依赖。

明确不为假设中的高并发或分布式场景提前设计复杂基础设施。

---

## 2. 最重要的架构原则：模块自治

7GRecorder 的业务不是一条必须全部成功的流水线。

正确模型：

```text
                    ┌─ Bilibili Archive（可选）
Recording + Local ──┼─ COS Storage（可选）
                    ├─ Songs / AI（可选）
                    └─ External Publisher（可选）
```

而不是：

```text
Recording → Bilibili → COS → Songs → NetEase → Cleanup
```

### 2.1 模块自治规则

每个模块：

1. 有自己的 enable/config；
2. 有自己的业务状态；
3. 有自己的 Job / reconciliation；
4. 只读取必要的 Recording/File 元数据；
5. 不直接修改其他模块的业务状态；
6. 未配置时完全不创建该模块的任务；
7. 失败时只记录本模块失败，不把 Recording 标记为失败；
8. 不要求其他可选模块成功后才继续工作。

允许共享的只有基础设施：

```text
SQLite
Filesystem
Scheduler / Worker Runtime
Resource Guard
```

这里的“模块独立”指**业务结果不互相作为成功前置条件**，不代表各模块可以无视同一台机器的物理资源。Resource Guard 可以因为直播优先级或磁盘 Critical 暂停/取消低优先级 Job；这种资源协调不改变模块各自的业务状态语义。

不引入通用 Event Bus、Workflow Engine 或 DAG 来“解耦”。

---

## 3. 技术栈

### Backend

```text
Go
GoFrame v2
GoFrame ghttp
GoFrame gdb + SQLite driver
SQLite
Goose（Migration）
```

### Frontend

```text
React
TypeScript
Vite
Tailwind CSS
shadcn/ui
React Router
TanStack Query
```

第一版不默认引入 Redux / Zustand；第一版实时状态使用 polling。

### External Tools / Integrations

```text
BililiveRecorder   多直播间录制、分段、弹幕
biliup             Bilibili 登录与投稿
FFmpeg / ffprobe   音视频处理
Tencent COS        可选对象存储 Adapter
whisper.cpp        后续可选 ASR
```

COS 第一版通过腾讯云官方 Go SDK：

```text
github.com/tencentyun/cos-go-sdk-v5
```

实现 Adapter，不增加独立常驻服务。

---

## 4. 应用形态

7GRecorder 是一个长期运行的 Web Backend Service。

第一版同一个 Go 进程承担：

```text
HTTP API / Auth
Recorder Webhook
Recording Orchestrator
Scheduler
Module Reconcilers
SQLite Job Queue
Workers
Storage Guard
Integration Adapters
```

第一版不拆 Web Server、Scheduler、Worker 多个自研服务。

代码层可保留以后支持：

```text
7grecorder all
7grecorder serve
7grecorder worker
7grecorder migrate
```

默认生产运行 `all`。

---

## 5. 总体结构

```text
                           Browser
                              │
                              ▼
                         Host Nginx
                      ┌───────┴────────┐
                      │                │
                      ▼                ▼
               React Static        /api/*
                                       │
                                       ▼
┌───────────────────────────────────────────────────────────────┐
│                    7GRecorder GoFrame                         │
│                                                               │
│ Auth / Account / Profile / Recording                          │
│                                                               │
│ Recorder Webhook ── Recording Orchestrator                    │
│                                                               │
│ Scheduler / Reconcilers                                       │
│   ├── Recorder Reconciler                                     │
│   ├── Local Storage Reconciler                                │
│   ├── Bilibili Reconciler                                     │
│   ├── COS Reconciler                                          │
│   └── Songs Reconciler                                        │
│                                                               │
│ SQLite Durable Job Queue ── Workers                           │
│                               ├── biliup                       │
│                               ├── FFmpeg                       │
│                               ├── COS Adapter                  │
│                               └── optional AI/Publisher        │
└───────────────────┬───────────────────────────────────────────┘
                    │ HTTP API
                    ▼
           ┌─────────────────────┐
           │ BililiveRecorder    │
           │ Room A / B / C ...  │
           └──────────┬──────────┘
                      │
                      ▼
                Shared Filesystem
```

核心区分：

- **Host shared service**：Nginx，负责同机多个网站的 TLS/反向代理/静态文件。
- **Project service**：7GRecorder、BililiveRecorder。
- **Tool**：biliup、FFmpeg、ffprobe、未来 whisper.cpp。
- **Optional integration**：Bilibili/COS/NetEase/AI；不配置即不运行对应模块。

普通 7GRecorder 发布只更新 Backend/Frontend，不重启 BililiveRecorder。

---

## 6. 模块边界

### 6.1 Recording Module

职责只有：

- Desired Room 配置；
- Recorder API / Webhook；
- Stream / Session / File / Recording 生命周期；
- 将完成分段可靠落到本地共享文件系统；
- 修复 Webhook 丢失和重启状态。

不负责：

- 决定 Bilibili 是否成功；
- 等待 COS；
- 等待 Songs；
- 长期保留原录像。

### 6.2 Local Storage Module

职责：

- 管理 `/data/7grecorder/recordings`；
- 统计 7GRecorder 录播文件实际占用；
- 执行本地滚动保留；
- 保护系统预留磁盘空间；
- 提供本地文件是否 AVAILABLE 的权威状态。

Local Storage **不以 Bilibili/COS/Songs 成功作为删除前置条件**。

### 6.3 Bilibili Archive Module

启用条件：Recording Profile 配置了启用的 Bilibili publishing profile 且 Credential 可用。

职责：

```text
扫描 COMPLETED Recording
→ 确认当前仍有可读本地视频文件
→ 创建 Upload Job
→ biliup
→ verify
→ 保存 BVID / publication status
```

失败只影响 `Publication`。

本地文件在上传前已经被滚动清理时，状态记录为 `SOURCE_MISSING`，不影响 Recording 和其他模块。第一版 Bilibili Module 只从 Local Source 投稿，不自动依赖 COS 回源；以后若确有需求再作为显式新能力设计。

### 6.4 COS Storage Module

启用条件：Recording Profile 配置了 COS Storage Profile 和 Credential。

职责：

```text
检测 CLOSED RecordingFile
→ 上传 COS
→ 保存 COS object metadata
→ 按配置的 managed quota 维护 COS 滚动存储
```

COS 可以按分段上传，不要求等待整场 Recording Finalize。

COS 删除只删除：

- 7GRecorder 自己管理的 Prefix；
- 数据库中明确登记的 Object；
- 最旧的已完成 Recording 对应对象。

绝不扫描并删除 Bucket 内未知对象。

COS 对象被滚动删除不影响 Bilibili、本地 Recording metadata 或 Songs。

### 6.5 Songs Module

只在启用时扫描 Recording，并在本地源文件仍可用时处理。

如果源录像已被 Local Storage 滚动删除：

```text
song_processing_status = SOURCE_MISSING / SKIPPED
```

第一版 Songs 只使用 Local Source，不自动从 COS/Bilibili 回源；不阻塞其他模块。

### 6.6 External Publisher Module

网易云等 Publisher 只处理已存在的 Song/Publication Item；平台失败不影响 Recording、Bilibili 或 COS。

---

## 7. Recording 生命周期

严格区分：

```text
Stream           主播直播状态
RecorderSession  BililiveRecorder 一次连续录制
RecordingFile    一个实际分段文件
Recording        用户看到的一场直播
```

### 7.1 Recording 聚合

```text
Recording
├── RecorderSession 1
│   ├── File 1
│   └── File 2
└── RecorderSession 2
    └── File 3
```

短暂断流后在 `finalize_grace_period` 内恢复，则继续同一个 Recording。

### 7.2 Recording 状态

只描述录制本身：

```text
ACTIVE
FINALIZING
COMPLETED
ABORTED
```

不因为 Bilibili/COS/AI 失败改变 Recording 状态。

### 7.3 SessionStarted

- 没有活动 Recording：创建 `ACTIVE` Recording + Session。
- 存在 grace period 内的 `FINALIZING` Recording：恢复为 `ACTIVE` 并新增 Session。
- 旧 Recording 已 `COMPLETED`：创建新 Recording。

### 7.4 SessionEnded

更新 Session；若没有其他 active Session：

```text
Recording → FINALIZING
finalize_at = ended_at + finalize_grace_period
```

Scheduler 到期后变为 `COMPLETED`。

### 7.5 Stream Event

`StreamStarted / StreamEnded` 只维护 Profile runtime 的直播状态，不直接创建/完成 Recording。

### 7.6 File Event

RecordingFile：

```text
WRITING
CLOSED
MISSING
DELETED
```

`FileClosed` 可以直接 UPSERT，不能假设 `FileOpening` 一定先到。

---

## 8. Profile Runtime

运行时状态与配置分开：

```text
stream_status:
  UNKNOWN | OFFLINE | LIVE

recorder_status:
  UNKNOWN | IDLE | RECORDING | ERROR

sync_status:
  SYNCED | PENDING | ERROR
```

Recording Profile 保存 Desired State；BililiveRecorder 是执行器。

Profile 修改：

```text
API 保存 SQLite
→ sync_status=PENDING
→ enqueue SYNC_RECORDER_PROFILE
→ 异步同步 BililiveRecorder
```

Recorder 暂时不可用不能阻止管理网站保存配置。

---

## 9. Recorder 集成

第一版一个共享 BililiveRecorder 实例管理多个 Room。

### 9.1 7GRecorder → Recorder

使用 HTTP API：

- Room 增删改；
- 录制设置同步；
- 状态查询；
- 必要时手动 start/stop；
- reconciliation。

### 9.2 Recorder → 7GRecorder

使用 Webhook v2：

- Stream Started/Ended；
- Session Started/Ended；
- File Opening/Closed。

Webhook Handler：

```text
Validate
→ event_id 幂等
→ 写 recorder_events
→ 更新 Recording/Session/File
→ Commit
→ HTTP 200
```

不直接执行上传、COS、FFmpeg 或 AI。


### 9.3 生产运行模式

BililiveRecorder 使用**持久 workdir 的标准 `run` 模式**。

虽然 BililiveRecorder 提供适合集成的 config-less portable mode，但 7GRecorder 生产默认不采用它，因为 Recorder 必须在 7GRecorder Backend 短暂部署/故障时仍保留 Room/AutoRecord 配置并继续独立录制。

关系：

```text
SQLite RecordingProfile     → 业务 Desired State
BililiveRecorder workdir    → Recorder 独立运行保障
reconciliation              → 恢复一致性
```

日常修改以 7GRecorder 为准；Recorder WebUI 仅作为 debug/运维入口。

### 9.4 网络与认证

BililiveRecorder API：

- 仅 Docker internal network/localhost；
- 不暴露公网；
- 建议启用 HTTP Basic；
- Backend 使用静态 integration credential 调用。

7GRecorder `/internal/v1/*` 不经公网 Nginx 暴露。

### 9.5 Webhook 与恢复

Webhook 是主通道，但不是唯一恢复手段。

Backend 重启后必须：

```text
Recorder reconciliation
+ runtime reconciliation
+ controlled filesystem reconciliation
```

以恢复短暂部署期间可能遗漏的状态。

外部 payload/CLI 行为不得凭代码作者记忆硬编码；固定版本后保存脱敏 fixture，详见 `INTEGRATIONS.md`。

---

## 10. SQLite Durable Job Queue

SQLite 同时承担：

- 业务数据库；
- Recorder event inbox；
- Durable Job Queue；
- Web Session。

不引入 Redis / RabbitMQ / Kafka。

### 10.1 不做 Job DAG

不设计：

```text
job_dependencies
workflow_nodes
workflow_edges
```

每个模块通过自己的 reconciliation 和状态变化创建下一步 Job。

模块内部允许自然串联，例如：

```text
Bilibili Upload SUCCEEDED
→ 创建 Verify Job
```

但不存在跨模块工作流，例如：

```text
Bilibili succeeded → COS → Songs → Cleanup
```

### 10.2 Job 状态

```text
PENDING
RUNNING
SUCCEEDED
FAILED
CANCELLED
```

Retry 使用原 Job：

```text
status=PENDING
attempts += 1
run_after=...
```

### 10.3 Job Resource Class

```text
LIGHT
NETWORK
MEDIA
AI
MAINTENANCE
```

默认：

```text
LIGHT        2-4
NETWORK      1
MEDIA        1
AI           1
MAINTENANCE  1
```

直播过程中默认不启动新的 NETWORK/MEDIA/AI 重任务；已经开始的任务不因为开播立即强杀，除非进入磁盘紧急保护。

### 10.4 Job 幂等

使用稳定 `business_key`，例如：

```text
recording:123:bilibili:upload
publication:45:bilibili:verify
file:456:cos:upload
song:987:cut
recording:123:local-cleanup
```

同一业务任务 Retry 不新增第二条有副作用 Job。

### 10.5 Stale Recovery

长任务保存：

```text
locked_at
heartbeat_at
locked_by
```

普通可重做 Job stale 后回 `PENDING`。

有外部不可确定副作用的发布任务 stale 后进入对应模块的 `AMBIGUOUS` 状态，而不是盲目重新提交。

---

## 11. 本地滚动存储

### 11.1 配置层级

第一版 Local Recording Quota 是**服务器全局配置**，只由 SUPER_ADMIN 设置，因为所有 Profile 共用同一物理磁盘。

核心配置：

```text
max_recording_bytes
min_system_free_bytes
cleanup_target_ratio
```

例如：

```text
max_recording_bytes = 40 GB
min_system_free_bytes = 10 GB
cleanup_target_ratio = 0.85
```

不使用整个 70GB 系统盘作为录播可用空间。

### 11.2 计入配额

只统计 7GRecorder 管理的原始 Recording 视频/弹幕等录播资产。

`temp/` 单独即时清理；Songs 文件较小，第一版不需要单独滚动配额，但仍受系统最小空闲空间保护。

### 11.3 滚动删除规则

当：

```text
recording usage > max_recording_bytes
```

或预测下一次写入会逼近：

```text
free disk < min_system_free_bytes
```

Storage Reconciler 按：

```text
Recording.completed_at ASC
```

从最旧开始删除本地原始录播，直到下降到 target。

正常自动删除条件只要求：

- Recording 已 `COMPLETED`；
- 文件不是 `WRITING`；
- 文件当前未被 RUNNING Job 使用；
- `local_protected=false`。

**不检查 Bilibili/COS/Songs 是否完成。**

### 11.4 Hard Safety

如果达到硬磁盘保护线，但所有旧文件都不可删除（例如全是 active/protected/in-use）：

1. 暂停启动新的重任务；
2. 暂停开始新的录制；
3. 标记 Storage `CRITICAL` 并在 Dashboard 告警；
4. 当前录制优先尝试正常继续，但如果系统剩余空间继续逼近绝对安全下限，允许停止录制以保护服务器其他服务。

这是唯一允许 Storage Guard 影响 Recording 的情况，因为物理磁盘容量属于不可绕过的基础资源约束。

### 11.5 Protect

`local_protected=true` 表示跳过普通滚动清理。

后台必须明确提示：大量 Protected 文件可能耗尽可录制空间。

---

## 12. COS 滚动存储

COS 是独立可选模块，不是 Local Storage 的前置或后置步骤。

### 12.1 配置

每个 Recording Profile 可以有一个 COS Storage Profile：

```text
enabled
credential_id
region
bucket
prefix
max_managed_bytes
```

未配置或 `enabled=false`：不创建 COS Job。

### 12.2 上传单位

优先以 `CLOSED RecordingFile` 为上传单位：

```text
FileClosed
→ COS Reconciler 发现未复制文件
→ UPLOAD_COS_OBJECT Job
```

这样不用等整场下播，也能降低本地文件被滚动删除前尚未上传的概率。

### 12.3 COS 配额

`max_managed_bytes` 是 7GRecorder 自己维护的逻辑滚动容量，不依赖 Bucket 是否存在平台级硬容量。

在上传新对象前，如果：

```text
managed_usage + incoming_size > max_managed_bytes
```

COS Module 删除自己 Prefix 下最旧 Recording 的已登记对象，直到空间满足。

删除 COS 对象：

- 不删除本地文件；
- 不删除 Bilibili 稿件；
- 不删除 Recording metadata；
- 不影响 Songs。

---

## 13. Bilibili Publication

Bilibili 模块只在完整 Recording `COMPLETED` 后工作。

Publication 状态：

```text
PENDING
UPLOADING
VERIFYING
VERIFIED
FAILED
AMBIGUOUS
SOURCE_MISSING
```

`AMBIGUOUS` 用于：外部平台可能已经产生副作用，但本地未成功保存 external_id 的情况。

`SOURCE_MISSING` 表示本地原录像已被滚动删除，无法继续当前投稿。

Bilibili `VERIFIED` 不是 Local/COS 清理的必要条件。

---

## 14. 文件可用位置

管理网站应分别展示同一 Recording 当前有哪些来源：

```text
Local:      AVAILABLE / PARTIAL / DELETED
COS:        AVAILABLE / PARTIAL / NONE / DELETED / FAILED
Bilibili:   VERIFIED / ...
```

其中：

- Local/COS 是文件副本；
- Bilibili 是平台处理后的观看归档，不视为原文件 bit-for-bit copy。

管理后台下载：

- Local：使用受鉴权的下载接口，生产可通过 Nginx internal/X-Accel-Redirect 发送文件，避免 Go 进程复制大文件；
- COS：后端生成短期签名 URL；
- Bilibili：提供外部稿件链接，必要时由用户自行下载历史归档。

---

## 15. 前后端 API 边界

三类 API：

### Admin

```text
/api/v1/*
```

必须登录并进行 Ownership/Policy 检查。第一版资源边界建议：

```text
/api/v1/auth/*
/api/v1/accounts/*
/api/v1/recording-profiles/*
/api/v1/recordings/*
/api/v1/recordings/{id}/files
/api/v1/publications/*
/api/v1/storage/local
/api/v1/storage/cos/*
/api/v1/songs/*
/api/v1/jobs/*
/api/v1/credentials/*
/api/v1/system/health
```

### Public

```text
/api/public/v1/*
```

只返回明确公开 DTO，不复用 Admin DTO。

### Internal

```text
/internal/v1/*
```

供 BililiveRecorder/内部服务调用，不作为公开业务 API。

动作使用显式 Action Endpoint，不允许前端直接 PATCH 任意业务状态：

```text
POST /api/v1/jobs/{id}/actions/retry
POST /api/v1/recordings/{id}/actions/protect-local
POST /api/v1/recordings/{id}/actions/unprotect-local
POST /api/v1/recordings/{id}/actions/delete-local
```

Credential Secret 只写不读，API 永远不回显。

统一错误格式、分页、OpenAPI、Local/COS 下载等具体约定见 `API_DESIGN.md`。

---

## 16. 前后端分离与部署

Backend 提供 REST JSON + OpenAPI。

Frontend：

```text
/admin/*       管理后台
/@streamer/*   主播个人展示页面
```

生产保持同域：

```text
/       → React dist
/api/   → GoFrame
```

前后端代码分离，但 Monorepo，不拆两个 Git Repository。

---

## 17. Authentication

第一版：

```text
username/password
→ SQLite server-side session
→ Secure + HttpOnly + SameSite Cookie
```

State-changing Admin API 有 CSRF 防护。

不使用 JWT + Refresh Token/OAuth。

---

## 18. 文件系统

```text
/data/7grecorder/
├── db/
├── recordings/<profile-id>/<recording-id>/...
├── songs/<profile-id>/...
├── temp/<profile-id>/<job-id>/...
└── backups/db/
```

BililiveRecorder、7GRecorder、biliup、FFmpeg 使用相同挂载路径。

媒体 binary 不进入 SQLite。

---

## 19. Deployment

第一版：

```text
Host Nginx
Docker Compose:
  - 7grecorder
  - bililiverecorder
```

其中：

- Nginx 作为宿主机共享 TLS/静态文件/反向代理，便于同机运行其他网站；
- `biliup / ffmpeg / ffprobe` 进入 7GRecorder Runtime Image；
- React 在 GitHub Actions 构建成静态 `dist`，生产无需 Node Server；
- BililiveRecorder API 不暴露公网；
- `/internal/*` 不通过 Nginx 对公网代理；
- Backend 发布默认不 recreate/restart Recorder。

CI/CD：

```text
dev  → CI only
main → CI → GitHub Actions immutable release → SSH production deploy
```

不建立测试环境。

详细 release、migration、backup、rollback、server path 约定见 `DEPLOYMENT.md`；运行配置、Secret、Storage Guard、日志与恢复见 `OPERATIONS.md`。

---

## 20. 配置与运行边界

配置分成两层：

### Static Runtime Config

服务器文件/环境变量：

```text
APP_LISTEN_ADDR
DATA_ROOT
SQLITE_PATH
RECORDER_BASE_URL
RECORDER_BASIC_*
MASTER_KEY_PATH
LOG_LEVEL
```

不进管理后台，不进 SQLite。

### Dynamic Business Config

SQLite + Admin：

```text
Profiles
Recording Settings
Bilibili/COS/Songs configs
Local Storage quota
encrypted Credentials
```

Database 保存媒体**相对路径**，运行时基于受控 root resolve。任何读取/删除/下载必须防止 path traversal/symlink escape。

外部依赖版本固定，不在生产自动跟随 `latest`。

---

## 21. 第一版明确不做

```text
Microservices
Redis
RabbitMQ
Kafka / NATS
PostgreSQL / MySQL
Kubernetes
Generic Event Bus
Generic Workflow Engine / Job DAG
Generic Repository Framework
复杂 RBAC/ABAC
JWT + Refresh Token auth
Node production server
WebSocket realtime infrastructure
自研 Bilibili 抓流
实时 Whisper
跨模块“全部成功才继续”的 Pipeline
```
