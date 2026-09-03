# 7GRecorder — AI Coding 开发流程

## 1. 目标

本流程借鉴 OVAIAM 的 design-first / schema-first / test-gated 思路，但为 7GRecorder 做轻量化处理。

目标是避免 AI Coding 常见问题：

- 代码先行导致架构漂移；
- 数据库 schema 与代码不一致；
- AI 为通过测试而削弱测试；
- 不同 Coding Agent 对目录边界理解不一致；
- 重复造中间件、接口和抽象；
- 可选模块之间形成不必要的硬依赖；
- 现有代码被错误当成产品规格。

## 2. 权威信息来源

按问题类型确定权威来源，而不是简单规定一个全局优先级。

- 产品需求与非目标：`REQUIREMENTS.md`
- 架构、模块边界、集成方式和技术约束：`ARCHITECTURE.md`
- 当前目标数据库结构：`DATABASE.md`
- 测试义务和 Release Gate：`TESTING.md`
- 当前阶段、实现顺序和验收：`DEVELOPMENT_PLAN.md`
- API 协议和前后端契约：`API_DESIGN.md`
- 外部工具/平台边界：`INTEGRATIONS.md`
- CI/CD、Release、Rollback：`DEPLOYMENT.md`
- 静态配置、Secret、Storage Guard、Backup/Recovery：`OPERATIONS.md`
- 历史变化：Git history / migrations / release notes

规则：

> Code is implementation, not specification.

## 3. 标准流程

### A. 讨论

任何较大的行为、架构、数据库或外部集成变化先讨论：

```text
需求
→ 边界
→ 非目标
→ 影响模块
→ 数据影响
→ 测试影响
```

小型 bug fix 和不改变行为的重构可以直接进入实现，但仍需遵守现有设计。

### B. 更新设计

如果结论改变当前设计：

1. 更新 `REQUIREMENTS.md` 或 `ARCHITECTURE.md`；
2. 删除已经被替代的旧描述，不在当前文档里堆叠互相冲突的历史方案；
3. 如果涉及数据库，继续更新 `DATABASE.md`；
4. 更新 `DEVELOPMENT_PLAN.md` 中受影响的 Phase/任务。

Pre-v1 阶段始终维护一套干净的“当前目标状态”文档，不创建大量 v0.1/v0.2 草稿。

### C. Schema First

数据库变化必须按以下顺序：

```text
DATABASE.md
→ migration
→ GoFrame DAO/DO/Entity 生成或更新
→ persistence adapter
→ application/domain
```

禁止：

```text
先改 Go struct
→ 让 migration 或 DB 跟着猜
```

### D. 实现

实现只覆盖当前已确认的设计。

边界原则：

- GoFrame HTTP / gdb / DAO / DO / Entity 留在 Framework / Adapter / Persistence 边界；
- 核心 Recording、Job、Song 等业务规则使用项目自己的类型；
- 简单模块保持简单，不强制套 `domain/application/ports/adapter` 四层；
- 只有存在真实替换边界的外部能力才定义 Port，例如 Recorder、BilibiliUploader、MediaProcessor、Publisher；
- 不创建“为了抽象而抽象”的 Generic Repository。
- Recording、Local Storage、Bilibili、COS、Songs、Publisher 之间不直接调用彼此的业务 Service；通过稳定数据状态和各自 Reconciler/Job 协作。
- 不引入跨模块 Workflow/DAG 来表达本可由模块自治解决的流程。

### E. 测试

- 新行为：新增测试；
- 修改行为：同步修改描述旧行为的测试并补充边界场景；
- Bug fix：尽量先增加能复现问题的 regression test；
- 数据库改动：增加 migration/schema/repository 验证；
- 文件清理、投稿、凭证等高风险路径必须重点测试。
- 修改一个可选模块时必须验证不会改变 Recording Core 或其他模块的状态语义。
- 外部 API/CLI 集成必须以仓库固定版本和脱敏 fixture 为依据，不允许 Coding Agent 凭模型记忆猜字段/输出。

### F. CI、分支与人工 Review

分支：

```text
dev  = 集成分支，CI only
main = 正式分支，CI 成功后 Production Deploy
```

合并前至少执行：

```text
backend format/vet/test/build
frontend lint/typecheck/test/build
migration/schema checks
```

只有 `main` push 可以触发正式部署；普通 Backend 发布不得连带重启 BililiveRecorder。

最终由人工 Review：

- 是否符合需求；
- 是否增加了无收益依赖；
- 是否存在数据/文件删除风险；
- 是否把框架类型泄漏进业务核心；
- 是否存在文档与代码漂移。

## 4. 依赖新增规则

任何新依赖先回答：

1. 标准库或现有依赖能否简单解决？
2. 是否真的需要常驻服务？
3. 是否显著增加部署、磁盘、内存或维护成本？
4. 是否只是为了潜在未来性能？

默认不增加：

- Redis
- RabbitMQ
- Kafka / NATS
- PostgreSQL / MySQL
- Elasticsearch
- Node production server
- Kubernetes
- 独立 Worker 服务

需要增加时先更新设计文档。

所有生产依赖固定版本；禁止为了“自动获得新特性”在 BililiveRecorder、biliup、Docker image 或 GitHub Actions 上使用漂移的 `latest`。

## 5. AI Coding 推荐任务粒度

每次给 Coding Agent 的任务尽量是一个可验证闭环，例如：

```text
实现 Recording Profile CRUD + 权限隔离 + 测试
```

而不是：

```text
把整个后台做完
```

推荐一个任务包含：

- 目标；
- 相关文档位置；
- 明确非目标；
- API / DB 影响；
- 验收标准；
- 必须执行的测试。

## 6. 文档维护规则

- Markdown 是权威格式。
- 当前文档描述“现在应该是什么”，不是 changelog。
- 不把每次 AI Coding 过程日志写进架构文档。
- 重要且长期有效的架构决策在 v1 后再按需引入 ADR；Pre-v1 不急于建立复杂 ADR 体系。
