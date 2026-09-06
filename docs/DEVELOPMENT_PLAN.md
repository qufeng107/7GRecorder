# 7GRecorder — Pre-v1 开发计划

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
- local download metadata；
- 相对路径 + root escape safety；
- Nginx internal/X-Accel-Redirect 下载；
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

### Reconciler / Jobs

- 扫描 completed + local source available；
- `UPLOAD_BILIBILI`；
- 当前先实现：扫描 `READY_TO_UPLOAD` Upload Source 并创建幂等 Publication/Job；
- pinned biliup version + CLI fixture；
- biliup CLI Adapter；
- `VERIFY_BILIBILI`；
- Publication status；
- AMBIGUOUS recovery；
- SOURCE_MISSING。

验收：

- 未配置时无任何 Bilibili Job；
- 上传失败不影响 Recording；
- 本地滚动删除不等待 Publication；
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
- 当前先实现：扫描 `READY_TO_UPLOAD` Upload Source 并创建幂等 COS Object/Job；
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

## 9. Phase 6 — Songs Manual MVP

- Song CRUD；
- manual start/end；
- FFmpeg Adapter；
- CUT_SONG_AUDIO；
- M4A；
- Confirm/Reject/Re-cut；
- HTML5 Audio；
- SOURCE_MISSING handling。

Songs 关闭或失败不影响其他模块。

---

## 10. Phase 7 — Operations & Recovery

- Jobs UI；
- Module status UI；
- local/COS usage Dashboard；
- credential health；
- Audit Log；
- SQLite scheduled backup；
- temp cleanup；
- manual retry/cancel；
- protect/unprotect；
- local/COS manual delete；
- restart/reconciliation drills；
- SQLite daily backup/restore drill；
- release rollback drill；
- old release/image/log/temp cleanup；
- master key backup checklist。

---

## 11. Phase 8 — AI Songs

- singing region detection；
- selective extraction；
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
