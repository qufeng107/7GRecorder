# 7GRecorder

7GRecorder 是一个面向多个 Bilibili 直播间的轻量自动录播、滚动存储、归档与歌曲整理平台。

系统运行在单台 Ubuntu 云服务器上，以普通观看者身份录制公开直播。每个 Recording Profile 都可以独立启用或关闭不同能力：

```text
直播录制 → 本地滚动存储
              │
              ├── 可选 Bilibili 自动归档
              ├── 可选 COS 滚动存储
              ├── 可选歌曲处理 / AI
              └── 可选网易云等外部发布
```

## 核心原则

- 自研后端统一使用 **Go + GoFrame v2**。
- 前后端分离，前端使用 **React + TypeScript + Vite**。
- 架构采用 **轻量模块化单体**：借鉴清晰边界，不追求目录数量和形式化 DDD。
- **模块自治、弱耦合**：Recording、Local Storage、Bilibili、COS、Songs、External Publisher 各自维护配置、状态与 Job；任何可选模块未配置、失败或停用，都不能阻塞其他模块。
- 7GRecorder 是业务控制中枢，不重新实现成熟的直播抓流、编解码和 Bilibili 上传底层。
- **BililiveRecorder** 是独立录制执行器；7GRecorder 通过 HTTP API 下发配置，通过 Webhook 接收事件。
- **biliup / FFmpeg / ffprobe** 作为 CLI 工具由 7GRecorder Worker 调用，不作为额外常驻服务。
- 数据库使用本机 **SQLite**；任务队列也使用 SQLite 持久化，不引入 Redis / RabbitMQ / Kafka。
- 媒体文件通过共享文件系统交互，不经过数据库或内部 HTTP 传输。
- 本地录播采用**容量上限 + 最小系统剩余空间保护 + 最旧优先滚动删除**，避免录播写满系统盘影响同机其他服务。
- COS 为可选的第二份近期文件滚动存储；只管理 7GRecorder 自己上传的对象，并按配置容量独立滚动删除最旧录播。
- Bilibili 是独立的长期观看归档通道，不作为本地/COS 清理的硬依赖。
- 第一版业务规则：一个管理账号最多管理一个 Recording Profile；数据模型从第一天按一个账号可拥有多个 Profile 设计。
- 第一版优先简单、稳定、低资源和易 AI Coding，不为假设中的高并发做过度设计。

## 推荐生产形态

```text
Host Nginx
  ├── /          → React 静态 release
  ├── /api/      → 127.0.0.1 → 7GRecorder
  └── internal media location → Local file download

Docker Compose
  ├── 7GRecorder
  │    ├── GoFrame API / Auth
  │    ├── Recorder Webhook
  │    ├── Recording Orchestrator
  │    ├── Scheduler / Reconciliation
  │    ├── SQLite Durable Job Queue
  │    └── Workers
  │         ├── biliup CLI
  │         ├── FFmpeg CLI
  │         ├── Tencent COS Go SDK
  │         └── optional AI / Publisher
  │
  └── BililiveRecorder
       └── 持久配置、多直播间独立录制

/data/7grecorder
  ├── db/
  ├── recordings/
  ├── songs/
  ├── temp/
  └── backups/
```

生产环境不需要 Node Server、Redis、PostgreSQL、外部消息队列或微服务基础设施。7GRecorder Backend 发布时默认不重启 BililiveRecorder。

## 文档

- [需求说明](docs/REQUIREMENTS.md)
- [技术架构](docs/ARCHITECTURE.md)
- [数据库设计](docs/DATABASE.md)
- [开发计划](docs/DEVELOPMENT_PLAN.md)
- [测试规范](docs/TESTING.md)
- [API 设计约定](docs/API_DESIGN.md)
- [外部集成边界](docs/INTEGRATIONS.md)
- [CI/CD 与生产部署](docs/DEPLOYMENT.md)
- [运行配置、运维与恢复](docs/OPERATIONS.md)
- [AI Coding 开发流程](docs/AI_DEVELOPMENT_WORKFLOW.md)
- [Coding Agent 入口规则](AGENTS.md)

## 当前阶段

当前仓库处于 Pre-v1 **Phase 0 工程初始化** 阶段，已具备 GoFrame Backend、React/Vite Frontend、SQLite migration、Docker Compose、GitHub Actions CI 与 main-only Production Deploy 的基础骨架。

架构、模块自治、Recording 生命周期、SQLite Job Queue、本地/COS 滚动存储、API 约定、外部集成、CI/CD、生产部署、备份/恢复和运行安全边界均已确定。

开发阶段仍需基于仓库固定的 BililiveRecorder/biliup 版本制作真实脱敏 fixture，并继续把 `DATABASE.md` / `API_DESIGN.md` 落成 DAO、DTO、OpenAPI 和业务实现；这些属于实现验证，不再是大的架构决策。

## 工程入口

```text
backend/                 GoFrame Backend
frontend/                React + TypeScript + Vite Frontend
backend/migrations/      Goose SQLite migrations
deploy/                  Compose 与 Nginx 示例
scripts/deploy/          生产部署 / 回滚脚本
.github/workflows/       CI 与 main-only 生产部署
testdata/                外部工具脱敏 fixture 目录
```

CI 当前负责：

```text
Backend: gofmt / vet / test / build
Frontend: lint / typecheck / test / build
System: docker compose config
```


## 开发前设计结论

当前 Pre-v1 设计可直接进入工程初始化。

已确定：

```text
GoFrame + SQLite
React/Vite
轻量模块化单体
BililiveRecorder 持久独立运行
SQLite Durable Job Queue
Local/COS 独立滚动存储
Bilibili/COS/Songs 模块自治
GitHub Actions CI
main-only SSH Production Deploy
SQLite backup + one-release migration compatibility
Host Nginx + X-Accel-Redirect large-file download
```

实现阶段不应再次引入新的大架构，除非真实外部工具验证证明当前设计无法满足需求。
