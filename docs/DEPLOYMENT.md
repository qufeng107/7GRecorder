# 7GRecorder — CI/CD 与生产部署设计

## 1. 目标

7GRecorder 不是企业级多环境系统，第一版只维护一台正式服务器。

分支：

```text
dev   → 日常集成/测试分支，不部署服务器
main  → 正式分支，CI 通过后自动部署正式服务器
```

不建立独立 test/staging 环境。

核心原则：

> **代码可以频繁部署，但 7GRecorder Backend 的部署不能无故重启正在录制的 BililiveRecorder。**

---

## 2. Git 分支流程

推荐：

```text
feature/* / fix/*
      ↓ PR
     dev
      ↓ CI
      ↓
release PR
      ↓
     main
      ↓ CI
      ↓
Production Deploy
```

规则：

- `dev` push：CI only；
- `main` push：CI + Production Deploy；
- Pull Request：CI；
- 不从 `dev` 自动部署任何服务器；
- 正式发布通过 `dev → main` 合并；
- Hotfix 可从 `main` 拉分支后合回 `main`，发布后再同步回 `dev`。

Pre-v1 不增加 release branch。

GitHub Repository 建议：

- `main` 禁止 force-push；
- `main` 合并前要求 CI status check；
- `dev` 作为集成分支，不承担生产部署语义；
- 不要求为了个人项目引入复杂 Environment approval 流程。

---

## 3. GitHub Actions

第一版两个 workflow 即可：

```text
.github/workflows/ci.yml
.github/workflows/deploy-prod.yml
```

### CI

触发：

```text
pull_request
push: dev
push: main
workflow_call
```

执行：

Backend：

```text
gofmt check
go vet
go test
go build
migration clean-db smoke
```

Frontend：

```text
pnpm install --frozen-lockfile
lint
typecheck
test
vite build
```

System：

```text
compose/config validation
关键静态配置检查
```

### Production Deploy

只触发：

```text
push: main
```

流程：

```text
CI
→ GitHub Runner 构建源码 release
→ checksum
→ SSH/SCP 到正式服务器
→ server build/install/deploy script
→ health check
```

Production Deploy 使用：

```text
concurrency:
  group: deploy-prod
  cancel-in-progress: false
```

避免两个正式部署相互覆盖。

---

## 4. SSH 模式

沿用 OVAIAM 的安全 SSH 思路：

GitHub Secrets / Variables 只保存部署连接信息：

```text
PROD_SSH_HOST
PROD_SSH_USER
PROD_SSH_PORT
PROD_SSH_PRIVATE_KEY
PROD_SSH_KNOWN_HOSTS
```

要求：

- 私钥只放 GitHub Secret；
- 使用 `StrictHostKeyChecking=yes`；
- known_hosts 固定，不使用 `ssh-keyscan` 在每次部署中动态信任；
- `BatchMode=yes`；
- SSH key 只用于部署服务器；
- Workflow 第三方 Action 尽量少，使用时固定 commit SHA，而不是浮动 tag。

**应用运行 Secret 不通过 GitHub Actions 下发。**

Bilibili/COS/网易云 Secret、master key 等保留在正式服务器，由应用数据库/服务器本地 Secret 管理。

---

## 5. Release Artifact

CI 在 GitHub-hosted Runner 完成，用于保证测试、类型检查和构建门禁通过。

生产 Release 不再从 GitHub Runner 上传 Docker image。由于 GitHub 到国内轻量服务器的 SCP 链路可能极慢，第一版生产部署上传源码小包，并在正式服务器上执行 Docker build 与前端静态构建。

约束：

- `main` 部署仍必须先通过 GitHub CI；
- 服务器构建只发生在 `main` 生产部署阶段；
- Runtime 仍不包含 Node production server；
- 构建依赖版本仍必须固定；
- 如果服务器资源压力影响录制，再改回 registry / 压缩 image / 对象存储分发等方案。

每个 Release 以 Git commit SHA 标识：

```text
<sha>
```

Release 包建议包含：

```text
7grecorder-release-<sha>.tar
├── source.tar
├── frontend/dist/
├── RELEASE_SHA
└── SHA256SUMS
```

Backend image：

- 多阶段构建；
- 包含 7GRecorder Go binary；
- 包含固定版本的 `biliup`、`ffmpeg`、`ffprobe`；
- 不包含 Node runtime；
- 运行时不下载依赖。

Server deploy builds:

```text
source.tar
→ docker build 7grecorder:<sha>
frontend/dist
→ releases/<sha>/frontend/dist
```

直接作为静态 release 内容。

所有 Release 文件生成 checksum。

---

## 6. 服务器目录

推荐：

```text
/opt/7grecorder/
├── releases/
│   ├── <sha-a>/
│   ├── <sha-b>/
│   └── <sha-c>/
├── current -> releases/<current-sha>
└── deploy/

/etc/7grecorder/
├── app.env
├── recorder.env
└── master.key

/data/7grecorder/
├── db/
├── recordings/
├── songs/
├── temp/
└── backups/
```

权限：

- `/etc/7grecorder/*` 只允许运行/运维账号读取；
- `master.key` 建议 mode `0640`，owner 为 `root`，group 为部署/运行账号所在组；
- `/data/7grecorder` 不随 Release 删除；
- Release Artifact 不包含生产数据和 Secret。

---

## 7. Nginx 与 Docker Compose

由于同一台服务器还可能运行其他网站，第一版推荐：

> **Nginx 作为宿主机共享反向代理/TLS 服务，不放进 7GRecorder Compose。**

7GRecorder `compose.yaml` 只管理：

```text
7grecorder
bililiverecorder
```

其中：

```text
7grecorder
→ host 127.0.0.1:<app-port>
```

供宿主机 Nginx 访问。

BililiveRecorder：

- 和 7GRecorder 位于同一 Docker network；
- API 不暴露公网；
- 如需要运维访问，只绑定 localhost 或使用 SSH tunnel；
- 使用持久 volume/workdir 保存自身配置。

Nginx：

```text
/                     → current/frontend/dist
/api/*                → 127.0.0.1:<app-port>
internal media path   → /data/7grecorder/...（internal only）
```

`/internal/*` 不配置公网反向代理。

---

## 8. BililiveRecorder 的部署隔离

BililiveRecorder 使用持久配置的 `run` 模式，而不是 config-less portable mode。

原因：

- 7GRecorder Backend 重启/部署时，Recorder 仍保留 Room 与 AutoRecord 配置；
- Backend 临时不可用不能导致录播模块同时失效；
- Backend 恢复后通过 HTTP API reconciliation 恢复 Desired/Actual State 一致性。

Production Deploy 默认只更新：

```text
7grecorder container
frontend dist
```

**不执行普通 `docker compose down`。**

**不因为每次 Backend 发布而 recreate/restart BililiveRecorder。**

BililiveRecorder 的升级是独立运维动作：

1. 固定新版本；
2. 确认没有正在录制，或明确接受中断；
3. 单独升级；
4. 验证 API/Webhook fixture 兼容；
5. 触发 reconciliation。

---

## 9. 部署顺序

正式部署建议：

```text
1. verify release checksum
2. preflight:
   - release SHA
   - Docker available
   - data dir writable
   - sufficient free disk
3. install release directory
4. build new 7GRecorder image on server
5. build frontend static dist on server
6. create SQLite online backup
7. gracefully stop only old 7GRecorder app
8. run new image migration command
9. start new 7GRecorder app
10. atomically switch frontend current release
11. health/readiness check
12. optional nginx config test/reload when config changed
13. mark release current
14. cleanup old release artifacts/images
```

Backend 停止期间 BililiveRecorder 继续录制。

短暂 Webhook 投递失败通过：

- Recorder 自身重试；
- Backend 启动后的 reconciliation；

恢复。

---

## 10. Database Migration

Goose migration forward-only。

为支持简单 Rollback，默认规则：

> **每个 Release 的 DB Migration 必须至少与“上一版应用”向后兼容。**

因此：

- 新增表/列优先；
- 不在同一个 Release 立即删除上一版仍需要的列；
- rename/drop 等破坏性修改使用“两阶段迁移”；
- migration 前必须备份 SQLite。

示例：

Release A：

```text
add new_column
application can read old + new
```

Release B：

```text
application only uses new_column
```

更后续 Release 才：

```text
drop old_column（若确实值得）
```

本项目数据量小，没有理由为了快速 schema 清理牺牲回滚安全。

---

## 11. Health Check

Backend 提供：

```text
GET /health/live
GET /health/ready
```

`live`：

- 进程正常即可。

`ready` 至少检查：

- SQLite 可读写；
- migration 版本正常；
- data/temp 目录可访问。

BililiveRecorder/COS/Bilibili 不作为 API readiness 的硬条件：

> 可选/外部模块故障不能把整个管理网站判为不可用。

它们在 Admin System Health 中单独展示。

Deployment 只有在新 Backend ready 后才成功。

---

## 12. Release 回滚

服务器保留最近若干个 Release，例如：

```text
3 个
```

并保留对应 7GRecorder Docker image tag：

```text
7grecorder:<git-sha>
```

Rollback：

```text
select previous SHA
→ switch backend image
→ switch frontend current
→ start
→ readiness check
```

正常 Rollback **不执行 down migration**。

由于 migration 要求上一版兼容，新旧应用可使用升级后的 DB schema。

如果未来确实需要不兼容 migration，必须单独设计数据恢复步骤，不允许 Deployment Script 自行猜测。

---

## 13. Release 清理

成功部署后：

- 保留最近 3 个 server release；
- 删除更旧的 release directory；
- 删除不再引用的旧 7GRecorder image；
- 不清理 BililiveRecorder image 当前版本；
- 不触碰 `/data/7grecorder`；
- GitHub Artifact 只需短期保留，例如 3–7 天。

避免长期积累 Docker image 和 release tar 占满 70GB 磁盘。

---

## 14. 配置与 Secret 不随部署覆盖

以下由服务器长期保留：

```text
/etc/7grecorder/app.env
/etc/7grecorder/recorder.env
/etc/7grecorder/master.key
/data/7grecorder/db/*
```

Deploy Script 不覆盖这些文件。

仓库只提供：

```text
.env.example
config example
```

新增必须配置的静态参数时：

- Release preflight 检查；
- 缺失则 fail deployment；
- 不用空默认值“带病启动”。

---

## 15. 版本固定

以下依赖禁止生产自动追 `latest`：

```text
BililiveRecorder
biliup
ffmpeg base/runtime image
Go toolchain
Node/pnpm
Go modules
npm packages
GitHub Actions
```

升级外部录播/上传工具视为独立变更，需要 Adapter fixture/test。

---

## 16. 第一版不做

```text
staging server
blue/green environment
Kubernetes rollout
container registry 必选依赖
ArgoCD
Terraform
多机部署
自动数据库 down migration
```

GitHub Actions + immutable artifact + SSH 已足够。
