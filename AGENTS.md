# 7GRecorder Coding Agent Instructions

本仓库采用 design-first、schema-first、test-gated 的 AI Coding 流程，但保持轻量，不为流程本身制造额外复杂度。

在修改生产代码前：

1. 阅读 `docs/AI_DEVELOPMENT_WORKFLOW.md`。
2. 阅读 `docs/ARCHITECTURE.md` 和 `docs/REQUIREMENTS.md`，确认当前目标行为与边界。
3. 如果涉及数据库或持久化，先阅读并更新 `docs/DATABASE.md`。
4. 阅读 `docs/TESTING.md`，确定需要新增或更新的测试。
5. 阅读 `docs/DEVELOPMENT_PLAN.md`，确认当前 Phase、范围和实现顺序。
6. 涉及 API/Frontend 时阅读 `docs/API_DESIGN.md`。
7. 涉及 BililiveRecorder、biliup、FFmpeg、COS 或 Publisher 时阅读 `docs/INTEGRATIONS.md`。
8. 涉及部署、配置、Secret、备份、日志或恢复时阅读 `docs/DEPLOYMENT.md` 与 `docs/OPERATIONS.md`。

## 不可违反的规则

- 先讨论和更新设计，再修改会改变架构或外部行为的生产代码。
- 数据库变更先更新 `DATABASE.md`，再写 migration、DAO 和 persistence 代码。
- 不从 Go struct、GoFrame DAO/DO/Entity、migration 或现有 SQLite 文件反推“权威设计”。
- 代码是实现，不是规格；代码与当前文档冲突时先解决规格冲突。
- 不为了让 CI 通过而删除、放宽、跳过或伪造测试。
- 不把 GoFrame、gdb、DAO/DO/Entity 类型扩散为 Domain/Application 的业务契约。
- 不为了形式上的 DDD 创建没有实际边界价值的目录、接口或抽象。
- 不新增 Redis、RabbitMQ、Kafka、PostgreSQL、微服务等基础设施，除非先通过设计讨论证明存在明确收益。
- 任何可能删除录像、重复投稿或暴露凭证的改动必须有明确的安全条件和测试。
- Recording、Local Storage、Bilibili、COS、Songs、Publisher 是独立业务边界；不得因为实现方便建立跨模块成功依赖或全局 Pipeline 状态。
- 可选模块未配置或失败时，必须保证 Recording Core 和其他模块仍可独立运行。
- 如果权威文档彼此冲突，暂停受影响实现并报告冲突，不要自行猜测。
- 外部工具/API 不允许凭模型记忆猜 payload/CLI 输出；以仓库固定版本和脱敏 fixture 为准。
- 生产依赖禁止自动追 `latest`；BililiveRecorder、biliup、FFmpeg、Go/Node、Actions 等版本升级要作为显式变更。
- `dev` 只跑 CI；只有 `main` push 在 CI 通过后可以触发正式部署。
- 普通 7GRecorder 发布不得通过 `docker compose down` 连带停止正在录制的 BililiveRecorder。

核心原则：**清晰边界优先于目录数量，简单可靠优先于架构炫技。**
