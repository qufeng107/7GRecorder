# 7GRecorder — Pre-v1 开发计划

> New-chat handoff: read `docs/CURRENT_STATUS.md` first for the current production SHA, recent upload-source repair
> work, known server state, and the immediate Bilibili/COS test plan. This development plan describes the broader
> phase roadmap.

## 1. 开发原则

```text
需求讨论
→ 更新 Design
→ 更新 Database
→ 更新 Testing
→ 小闭环实现
→ CI / Review
```

核心原则：

> **先完成可靠录制与本地滚动存储，再逐个增加可选模块；任何后续模块都不能反向污染 Recording Core。**

---

## 2. 设计状态与实现期验证

大的架构设计已经完成，可以进入工程初始化。

开发时仍需基于**固定外部组件版本**完成这些实现验证：

1. 用真实 BililiveRecorder pinned version 保存 Webhook/API 脱敏 fixture，并完成字段 mapping；
2. 用真实 biliup pinned version 保存 upload/verify 成功失败 fixture；
3. 把 `DATABASE.md` 的目标 Schema 落成 Goose migration；
4. 把 `API_DESIGN.md` 的资源边界落成 GoFrame DTO/OpenAPI；
5. 为腾讯 COS 官方 Go SDK 建立 fake/fixture adapter tests；
6. 根据实际服务器磁盘大小设置第一份 production local quota，不把示例值硬编码成产品规则。

这些属于实现验证，不再是新的架构决策。

---

## 3. Phase 0 — Project Bootstrap

### Backend

- GoFrame v2；
- SQLite / WAL；
- Goose Migration；
- config；
- structured logging；
- graceful shutdown；
- health/readiness；
- OpenAPI；
- common error/request-id middleware；
- API relative-path safety primitives；
- DAO generation workflow。

### Frontend

- React + TS + Vite；
- Tailwind + shadcn/ui；
- Router；
- TanStack Query；
- Admin/Public Layout；
- API Contract workflow。

### Repo / Deploy

- Makefile；
- `make ci`；
- `.github/workflows/ci.yml`；
- `.github/workflows/deploy-prod.yml`；
- `dev` CI only / `main` CI + Production Deploy；
- immutable Git-SHA release artifact；
- SSH pinned known_hosts；
- Dockerfile / compose；
- 宿主机 Nginx config；
- release/rollback/deploy scripts；
- `.env.example`；
- `/opt/7grecorder` / `/etc/7grecorder` / `/data/7grecorder` convention；
- health/live + health/ready。

Phase 0 只搭正式部署通道，不建设 test/staging server。

---

## 4. Phase 1 — Platform Foundation

- SUPER_ADMIN / MANAGER；
- no-default-password bootstrap admin CLI；
- server-side password reset CLI；
- SQLite Session / Cookie / CSRF；
- ManagerPolicy；
- Recording Profile CRUD；
- Profile 1:N schema + v1 max-one rule；
- Recording Settings；
- Basic Dashboard；
- Credential encryption foundation。

验收：Ownership/Policy 不越权。

---

## 5. Phase 2 — Recording Core

这一阶段只做 Recording，不引入 Bilibili/COS/Songs 成功条件。

### Recorder Adapter

- pinned BililiveRecorder version + fixtures；
- 持久 workdir `run` mode；
- HTTP API + internal Basic Auth；
- Desired State Sync；
- reconciliation；
- Backend deploy/restart 不停止 Recorder。

### Event Inbox

- Webhook endpoint；
- event_id 幂等；
- room → profile；
- fast commit。

### Lifecycle

- Stream Runtime；
- Recording ACTIVE/FINALIZING/COMPLETED/ABORTED；
- Session merge；
- RecordingFile WRITING/CLOSED/MISSING/DELETED；
- restart recovery。

验收：多个 Profile 并行、短断流归并、重复事件不重复。

---

## 6. Phase 3 — Local Rolling Storage + Durable Job Runtime

本地存储是第一个必须完成的运维闭环。

### SQLite Job Queue

- enqueue / atomic claim；
- business_key；
- priority；
- run_after；
- retry/backoff；
- heartbeat/stale recovery；
- 已实现目标：每次 Worker 启动使用唯一 lock identity；旧进程 Bilibili Job 冻结为 `AMBIGUOUS`，COS/本地可逆
  Job 自动恢复；部署使用 SQLite drain 阻止新 claim，并拒绝中断当前容器持有的任务；
- resource class semaphore；
- explicit resource IDs。

### Local Storage

- 全局 max recording bytes；
- min system free bytes；
- emergency floor；
- usage calculation；
- oldest completed rolling cleanup；
- protected / in-use skip；
- hard safety behavior；
- local file metadata for admin visibility；
- 相对路径 + root escape safety；
- local files are not the default download outlet; upload-source downloads use COS signed URLs；
- Dashboard quota/status。

验收：

```text
录播持续产生文件
→ 达到模拟 quota
→ 自动删除最旧 completed recording
→ 不删除 active/writing/in-use/protected
→ 不检查 Bilibili/COS/Songs 状态
```

---

## 7. Phase 4 — Bilibili Archive Module

这是第一个真正可选模块。

### Config / Credential

- Bilibili Publishing Profile；
- encrypted Credential；
- verify/update；
- enabled detection。

Current implementation note:

- Real upload now uses the pinned `biliup==1.2.4` CLI adapter from the worker process.
- The adapter writes decrypted biliup cookie JSON only to a per-job temp directory, invokes `biliup upload --submit`,
  then removes the temp directory.
- Successful CLI exit is treated as a successful publication even when the BV id is not present in stdout, so retry
  handling does not accidentally submit duplicates.
- Verification/listing after upload is still pending and should fill missing BV URLs later.

### Reconciler / Jobs

- 扫描 completed + local source available；
- `UPLOAD_BILIBILI`；
- 当前先实现：扫描 `READY_TO_UPLOAD` Upload Source 并创建幂等 Publication/Job；
- 已实现：Bilibili 标题/简介模板配置、上传请求快照生成和 fake uploader Worker 路径；
- pinned biliup version + CLI fixture；
- biliup CLI Adapter；
- Bilibili 多 P 投稿使用 Upload Source output parts，投稿标题/简介来自可编辑模板；
- `VERIFY_BILIBILI`；
- Publication status；
- AMBIGUOUS recovery；
- AMBIGUOUS 人工确认后重试，复用原 Publication/Job；
- SOURCE_MISSING。

验收：

- 未配置时无任何 Bilibili Job；
- 上传失败不影响 Recording；
- 本地业务视频滚动删除等待所有已启用远端目的地确认成功；模块失败不改变 Recording 状态，但会阻止该 Upload Source 被回收；
- retry 不重复投稿。

---

## 8. Phase 5 — COS Rolling Storage Module

### Config

- Tencent COS official Go SDK Adapter；
- COS Credential；
- region/bucket/prefix；
- `max_managed_bytes`；
- enable/disable。

### Reconciler / Jobs

- CLOSED File detection；
- `UPLOAD_COS_OBJECT`；
- 已实现：扫描 `READY_TO_UPLOAD` Upload Source 并创建幂等 COS Object/Job；
- 已实现：Worker 使用官方 Go SDK 上传 Upload Source 到 COS 并更新对象状态；
- 已实现：Upload Source 封装分片，按 COS/Bilibili 较小限制生成平台可消费文件；
- 已实现：Upload Source 发现会在合并窗口内存在同配置相邻 `ACTIVE`/`WRITING` 录像时暂缓生成父视频，避免直播文件轮转被过早固化成多个父视频；
- 已实现：Upload Source 发现不再把可能因丢失事件而陈旧的 Profile `LIVE/RECORDING` 运行态作为永久门禁；超过合并窗口后，以 `ACTIVE` Recording、`WRITING` video 和近期相邻录像文件作为是否仍在录制的安全依据；
- 已实现：将 `UPLOAD_MAX_PART_BYTES` 默认值从早期 4 GiB 调整到更保守的 3.8GB 级别，避免超过 Bilibili
  单文件上传边界；
- 当前修正：COS 直接上传每个 output part，不再生成 MP4 转码或 ZIP 派生文件；
- 当前修正：新 COS 视频对象使用 `videos/YYYY-MM-DD/session-NN/pNN.<source-format>`，历史对象不迁移；
- 已实现：COS 对象保留原始大小、上传对象大小及兼容历史对象所需的压缩 metadata；
- 当前开发：原始弹幕文件按 `recording_files.kind = 'danmaku'` 扫描入库，使用 `UPLOAD_COS_RECORDING_FILE`
  任务上传到 COS `raw/` 前缀；先归档原文，不解析、不对齐时间轴；
- object metadata；
- per-profile managed usage；
- oldest Recording COS rolling deletion；
- SOURCE_MISSING；
- signed download URL。

安全要求：

- 只管理自己的 Prefix；
- 只删除 DB 登记对象；
- 不触碰 Bucket 其他文件。

验收：

```text
COS disabled → no jobs
COS enabled → closed file copied
quota exceeded → oldest managed recording removed
local/bilibili unchanged
```

---

## 9. Phase 6 — Manual COS Songs V1

Status: design approved, implementation paused while operations work has priority. The existing deployed ACRCloud
checkpoint remains legacy behavior; do not add local model dependencies or change production Runs until this phase is
explicitly resumed.

实现顺序：

1. 对 CPU-only PANNs MobileNetV2/Cnn6 做离线对比，固定候选模型版本、checksum、license、标准化输出、
   脱敏 fixture 以及磁盘/内存/耗时实测；轻量模型未达标时才评估 Cnn14；
2. analysis/evidence/artifact/cache/reservation schema；
3. 全局托管空间统计、事务性空间预留、租约和 LRU cache safety；
4. AVAILABLE upload-source COS 视频选择器与可取消流式下载；
5. 本地窗口化推理、可恢复 chunk 状态与高召回时间聚合；
6. 允许 title/artist 为空的 Song Draft、边界版本；
7. 从原视频自动切 M4A、上传 COS、缓存播放；
8. Songs 管理列表、试听、填写元数据、编辑边界、Confirm/Reject；
9. 人工触发准确 MP4 导出和 5GB LRU 下载缓存；
10. 小文件生产验收并调优阈值后再允许大文件 Run。

V1 一次只分析一个 COS output，不自动扫描 Recording，不跨 output 聚合。Songs 失败不影响其他模块。

当前已部署 checkpoint 完成步骤 2、4，并曾以 ACRCloud 单分析文件实现旧版步骤 5-7 的最短闭环，同时提供
列表与本地缓存试听。该外部付费路径不再是目标方案。下一 checkpoint 先完成步骤 1、5、6，使新 Run 默认使用
无需凭证的本地候选检测；之后继续 cache-miss COS 回填、可编辑边界、Confirm/Reject 和步骤 9。检测推理运行
在独立 `AI` worker slot，不占用录播合并的 `MEDIA` slot，直播期间不启动新任务。

---

## 10. Phase 7 — Operations & Recovery

- Jobs UI；
- Module status UI；
- local/COS usage Dashboard；
- credential health；
- Audit Log；
- SQLite scheduled backup；
- temp cleanup；
- deploy-time disk cleanup；
- daily disk housekeeping timer；
- manual retry/cancel；
- protect/unprotect；
- local/COS manual delete；
- restart/reconciliation drills；
- SQLite daily backup/restore drill；
- release rollback drill；
- old release/image/log/temp cleanup；
- stale Docker build cache cleanup；
- old DB backup retention cleanup；
- master key backup checklist。

---

## 11. Phase 8 — Songs Evidence Enhancements

- automatic title/artist suggestions；
- original-versus-cover suggestions；
- local reference catalog；
- whisper.cpp Adapter；
- danmaku/lyrics evidence；
- Song Candidates；
- review flow。

AI resource class = lowest priority；直播期间默认不启动新 AI Job。

---

## 12. Phase 9 — External Publisher

网易云等：

- Credential Adapter；
- manual publish；
- idempotency；
- external status；
- failure isolation。

---

## 13. Phase 10 — Public / Creative Frontend

当前前端迭代优先完成模块化基础，再推进公开宣传页；保留 React/TypeScript/Vite。
本地环境、共享布局与全部现有控制台模块已完成迁移；后续推进公开页面及交互细化。当前范围与启动方式见 `FRONTEND_DEVELOPMENT.md`。顺序：

1. 本地工具链、锁文件、合成数据模式、隔离联调配置与浏览器 smoke；
2. 共享 API 契约、控制台布局、独立页面路由，以 Jobs 为首个迁移候选；
3. 统一 UI/主题，逐步迁移录像、上传、歌曲、账号和系统模块，保持业务与权限语义；
4. 公开主播页面独立视觉与动画原型、移动端与减弱动画支持；
5. 明确公开页 SEO/预渲染及公开数据接入。

范围与验收输入见 `FRONTEND_UI.md`。隔离环境先本地可复现；远程 staging 未部署，`dev` 仍仅 CI。

```text
/@streamer
/@streamer/songs
/@streamer/recordings
```

允许独立视觉、动画、创意播放器；依赖按需求增加。

---

## 14. AI Coding 任务粒度

推荐：

```text
实现 COS Reconciler：发现 CLOSED file、创建幂等 Job、SOURCE_MISSING + tests
```

不推荐：

```text
完成 COS 模块
```

每个任务写清：

```text
Goal
Relevant docs
Scope
Non-goals
DB impact
API impact
Module isolation impact
Tests
Acceptance criteria
```

---

## 15. Dependency Policy

默认不引入：

```text
Redis
RabbitMQ
Kafka/NATS
PostgreSQL/MySQL
Kubernetes
Elasticsearch
Workflow Engine
Generic Event Bus
Node production server
Redux
WebSocket infrastructure
```

新增依赖必须解决当前真实问题。

---

## 16. Completed Increment: Upload Review And Editing (2026-09-12)

Completed scope:

- show active recordings before upload-source completion;
- persist review requests on recordings and upload sources;
- freeze pending/running Bilibili and COS work without stopping BililiveRecorder;
- preserve a database-level late-completion guard;
- allow reviewed local publish-part download;
- accept parent-timeline deletion ranges and generate replacement `edited/...` parts;
- require explicit approval after edit verification;
- resume Bilibili and COS independently from the current output manifest.

The completed increment does not include browser video preview, a graphical timeline editor, frame-accurate cuts, or
automatic detection of content to remove. Those are optional future UI/media increments and must preserve the same
review gate and current-output-manifest rules.

---

## 17. Validated Increment: Concise Bilibili Part Titles (2026-09-12)

Validated on `dev` at `e4686395f1579bb578ff59fafda8b503a6abb806`; production release is intentionally pending
until no Bilibili/COS/media worker is active.

Completed scope:

- present Bilibili multipart titles as `p01`, `p02`, `p03`, and so on;
- create lightweight per-job symlink aliases instead of copying or renaming large media files;
- preserve canonical local output names, database relative paths, and COS object keys;
- remove aliases with the restricted biliup credential/work directory;
- cover alias naming and original-target resolution with the fake biliup adapter test.

This increment changes only future Bilibili submissions started after deployment. It does not rewrite an existing
submission or alter COS archive naming.

The deployed media compatibility fix first partitioned consecutive source segments at FFprobe stream-signature
changes. Production `06f27e3e` retains that behavior as a fallback but treats dimensions-only live PK changes
specially: normalize onto the dominant source resolution with aspect-ratio-preserving black padding, then return to
the normal two-hour/size-aware part policy. Other stream changes still create compatibility boundaries.

---

## 18. Current Increment: Production Domain And TLS (2026-09-13)

- serve `7g.chat` and `www.7g.chat` through host Nginx with HTTP-to-HTTPS redirect;
- add SUPER_ADMIN site TLS settings and encrypted Tencent SSL credentials;
- periodically discover and download the newest matching issued certificate;
- validate and stage certificate material without giving the app permission to reload host Nginx;
- install through a root-owned idempotent host service with rollback on validation or `nginx -t` failure;
- surface staged/deployed IDs, expiry, timestamps, and errors in the System page.

DNS mutation, certificate purchasing, and replacing Tencent Cloud's renewal lifecycle are non-goals.
