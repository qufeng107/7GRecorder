# 7GRecorder — 需求说明

## 1. 项目定位

7GRecorder 是一个运行在单台 Ubuntu 云服务器上的**多账号、多直播间自动录播、滚动存储与内容整理平台**。

系统以普通观看者身份监听配置的 Bilibili 公开直播间，并围绕一份可靠的本地 Recording 提供多个独立可选模块：

```text
直播间监听
  → 自动录制
  → 本地滚动存储
       │
       ├── 可选：自动归档 Bilibili
       ├── 可选：COS 近期文件滚动存储
       ├── 可选：歌曲整理 / AI
       └── 可选：网易云等外部发布
```

### 1.1 模块独立性是产品需求

Recording 是基础能力；其余能力均为可选模块。

要求：

- 没有配置 Bilibili：仍正常录制和本地滚动存储；
- Bilibili 上传失败：不影响 COS、Songs 和下一场录制；
- 没有配置 COS：完全不创建 COS 任务；
- COS 上传/删除失败：不影响本地和 Bilibili；
- AI/Songs 失败：不影响完整录播归档；
- 网易云失效：不影响 Songs 本地播放；
- 本地滚动清理不等待 Bilibili/COS/Songs 成功。

可选模块只共享 Recording/File 元数据和基础 Job Runtime，不形成跨模块 Pipeline。

模块独立指业务结果不互相依赖；同机 CPU、网络和磁盘仍由全局 Resource Guard 协调，以确保 Recording 和系统盘安全优先。

---

## 2. 技术约束

- 自研后端：**Go + GoFrame v2**。
- 前端：**React + TypeScript + Vite**。
- 前后端分离，但生产同域。
- 本机 **SQLite** 保存网站/业务数据与 Durable Job Queue。
- 不引入 Redis、RabbitMQ、Kafka、PostgreSQL 等无明确收益的中间件。
- BililiveRecorder 负责底层录制。
- biliup 负责 Bilibili 上传。
- FFmpeg/ffprobe 负责媒体处理。
- COS 通过独立 Adapter 访问，不增加常驻中间服务。

---

## 3. 运行环境与资源原则

服务器：

- 腾讯云轻量应用服务器；
- 上海；
- Ubuntu 24.04 LTS；
- 2 Core / 4 GB RAM；
- 70 GB SSD；
- 公网出带宽 6 Mbps；
- 同机还会运行小网站、API、自动化工作流。

优先级：

```text
系统磁盘安全
> 正在进行的录制
> 文件完整性
> 管理后台
> Bilibili/COS 网络任务
> FFmpeg
> AI
```

不允许录播写满系统盘影响其他服务。

---

## 4. 用户与直播间

角色：

```text
SUPER_ADMIN
MANAGER
```

### SUPER_ADMIN

可：

- 管理全部 Manager；
- 管理全部 Recording Profile；
- 设置 Manager 自助修改权限；
- 管理系统级 Local Storage Quota；
- 管理全部模块配置、任务、凭证和状态；
- 执行高风险运维动作。

### MANAGER

默认可以：

- 查看自己的 Profile；
- 修改自己的直播间和录制配置；
- 配置自己的 Bilibili/COS/网易云等可选模块；
- 查看自己的 Recordings/Songs/Jobs；
- 在权限允许时 Protect/下载自己的近期文件。

SUPER_ADMIN 可通过 ManagerPolicy 分别禁止：

```text
修改 Recording Profile/录制配置
修改自己的 Bilibili 模块与 Credential
修改自己的 COS 模块与 Credential
修改自己的网易云模块与 Credential
Protect/主动删除自己的本地文件
```

系统级 Local Storage Quota、硬安全线、全局运维设置始终只允许 SUPER_ADMIN 修改，不通过 ManagerPolicy 下放。

### 账号与 Profile

第一版产品规则：

```text
1 Manager → 最多 1 Recording Profile → 1 Live Room
```

数据库从第一天按：

```text
1 Manager → N Recording Profiles
```

设计。

同一个 active Bilibili Room 不允许属于两个 Profile。

---

## 5. Recording Profile

每个 Profile 至少有：

### 基础

```text
name
streamer_name
room_id
streamer_uid
timezone
enabled
public_enabled/public_slug
```

### Recording Settings

```text
auto_record
quality
record_danmaku
segment_duration
finalize_grace_period
```

正在录制/Finalizing 时第一版禁止修改 Room ID 和录制核心参数；下一场再修改。

`enabled=false` 表示不再开始新的自动录制，不等同于强制停止正在进行的 Recording。

---

## 6. Recording 生命周期

严格区分：

```text
Stream
Recorder Session
Recording File
Recording
```

### Recording 状态

```text
ACTIVE
FINALIZING
COMPLETED
ABORTED
```

短断流在 `finalize_grace_period` 内恢复时归入同一 Recording。

Recording 状态只表示“录制本身”，不因为上传、COS 或 AI 失败而失败。

### Recording File 状态

```text
WRITING
CLOSED
MISSING
DELETED
```

只有 CLOSED 文件才允许被上传/分析。

---

## 7. Local Storage Module — 本地滚动存储

这是 Recording 的基础存储能力，始终启用。

### 7.1 系统级容量配置

SUPER_ADMIN 在管理后台设置：

```text
Local recording max size
Minimum system free space
Cleanup target
Emergency free-space floor
```

例如 70GB 系统盘可以只给录播原文件 35–45GB 的预算，并额外保留系统安全空间。

### 7.2 滚动规则

录制文件先落本地。

当本地 Recording 文件总量达到上限或系统空闲空间不足：

```text
按 completed_at 从旧到新
→ 删除最早的可删除 Recording 原文件
→ 一直清理到目标容量以下
```

正常自动删除以**整场 Recording** 为单位。要求：

- Recording 已完成；
- 该 Recording 没有文件仍在写入；
- 没有 RUNNING Job 正在使用该 Recording 任一 File；
- 未被人工 `local_protected`。

只要一条文件不满足安全条件，就跳过整场 Recording；普通滚动清理不主动制造半场缺失。

**不要求：**

- 已上传 Bilibili；
- 已上传 COS；
- AI 已完成；
- Songs 已完成。

这是保证模块独立性的核心规则。

### 7.3 极端空间不足

如果硬安全线已经达到但没有可滚动删除的旧文件：

- 暂停新重任务；
- 暂停开始新录制；
- Dashboard 显示 Critical；
- 若继续写入会威胁系统盘，允许停止当前录制以保护同机其他服务。

### 7.4 Protect

Manager/SUPER_ADMIN 可以把重要 Recording 标记为本地保护。

后台必须显示其占用，并提示过多 Protected 文件会降低可用录播容量。

---

## 8. Bilibili Archive Module — 独立可选

启用条件：

```text
Bilibili publishing enabled
AND valid/configured credential
```

否则整个模块对该 Profile 不运行。

行为：

```text
发现 READY_TO_UPLOAD Upload Source
→ 检查上传源视频文件仍 Local AVAILABLE
→ 自动生成 Upload Job
→ biliup 上传
→ verify
→ 保存 BVID
```

Publication 状态独立：

```text
PENDING
UPLOADING
VERIFYING
VERIFIED
FAILED
AMBIGUOUS
SOURCE_MISSING
```

要求：

- 多分段先按 Upload Source 合并后作为一个稿件；
- 全局上传并发 1；
- 重试幂等；
- 进程中断后不得盲目重复投稿；
- 任一必要视频分段已经滚动删除时标记 `SOURCE_MISSING`，第一版不投稿残缺半场；
- Bilibili 成功/失败都不改变 Recording 完成状态；
- Bilibili 状态不决定本地文件是否可以滚动删除；
- 第一版只从 Local Source 投稿，不自动从 COS 回源。

Bilibili 是长期观看归档，不视为原始文件 bit-for-bit 备份。

---

## 9. COS Storage Module — 独立可选

用途：

> 保存一段时间内的近期原始录播文件，方便管理网站直接下载；更早的内容可以依赖已存在的 Bilibili 归档继续观看/自行下载。

启用条件：

```text
COS storage enabled
AND credential/bucket/region configured
```

否则不运行。

### 9.1 上传

COS 优先对 `READY_TO_UPLOAD` Upload Source 自动上传，和 Bilibili 共享同一份可上传视频边界。

需要记录：

```text
bucket
object_key
size
checksum(optional)
status
uploaded_at
```

### 9.2 COS 滚动容量

每个 COS Storage Profile 配置：

```text
max_managed_bytes
```

这是 7GRecorder 自己管理的逻辑容量。

新对象上传前，如果：

```text
当前 managed usage + 新文件大小 > max_managed_bytes
```

则删除 COS 中最早 Recording 的已管理对象，直到有足够容量。

要求：

- 只删除 7GRecorder 自己上传并登记的对象；
- 只操作配置的 Prefix；
- 不删除 Bucket 中未知文件；
- COS 滚动删除不删除本地文件；
- COS 失败不影响 Bilibili；
- 本地 Upload Source 文件已滚动删除而尚未上传时，COS 任务记 `SOURCE_MISSING`。

---

## 10. 管理网站中的文件来源

Recording 详情要分别显示：

```text
Local
COS
Bilibili
```

示例：

```text
Local      AVAILABLE     8.3 GB
COS        AVAILABLE     8.3 GB
Bilibili   VERIFIED      BVxxxx
```

或者：

```text
Local      DELETED
COS        AVAILABLE
Bilibili   VERIFIED
```

下载行为：

- Local：管理后台鉴权后下载；
- COS：生成短期签名下载 URL；
- Bilibili：打开已归档稿件，不伪装成原文件备份。

Public 主播页面默认不暴露原始 Local/COS 下载地址，除非以后明确增加公开下载功能。

---

## 11. Songs 基础功能

Songs 是独立可选模块。

关闭时：

```text
song_processing_status = DISABLED
```

人工 MVP：

```text
Recording
→ 人工 start/end
→ FFmpeg M4A
→ 浏览器播放
→ Confirm/Reject
```

AI 后续增强：

```text
歌曲区间检测
→ Unknown Song Draft
→ selective ASR / 弹幕 / 歌词证据
→ Candidates
→ 人工确认
```

如果原始 Recording 已被本地滚动删除且没有适用源文件：

```text
SOURCE_MISSING / SKIPPED
```

不影响其他模块。第一版 Songs 只使用 Local Source，不自动依赖 COS/Bilibili 回源。

---

## 12. 网易云等 External Publisher

独立 Adapter。

第一版默认人工发布已确认 Song。

未配置/平台 API 失效：只影响该 Publisher，不影响 Songs、Recording、Bilibili、COS。

---

## 13. Job / Scheduler 需求

需要 Durable Job Queue，但不需要外部 MQ。

SQLite Job 状态：

```text
PENDING
RUNNING
SUCCEEDED
FAILED
CANCELLED
```

不做通用 DAG。

模块内部自己 reconcile：

```text
Bilibili Reconciler
COS Reconciler
Songs Reconciler
Local Storage Reconciler
Recorder Reconciler
```

各自根据当前状态发现“应该做但尚未做”的工作并 enqueue Job。

### Resource Class

```text
LIGHT
NETWORK
MEDIA
AI
MAINTENANCE
```

默认重任务并发低，直播时不启动新的 NETWORK/MEDIA/AI 重任务。

Storage Critical 可以停止/取消低优先级重任务以保护磁盘。

---

## 14. Web 前端

技术：

```text
React + TypeScript + Vite
Tailwind CSS + shadcn/ui
React Router
TanStack Query
```

页面：

```text
/admin/*
/@streamer/*
```

### Dashboard

至少显示：

- Profile Live/Recorder 状态；
- 本地录播占用 / 配额 / 系统剩余空间；
- COS 各 Profile managed usage / quota；
- Bilibili/COS/Songs 模块启用状态；
- 当前 Jobs；
- 最近错误；
- Storage Warning/Critical。

后台信息架构：

- Overview 显示系统健康、模块状态和发布版本等只读概览；
- Recording Profiles 是账号作用域内的直播间与录制配置入口，普通 Manager 只能维护自己的 Profile，SUPER_ADMIN 可以跨账号管理；
- Recordings 是账号作用域内的录像索引、下载和本地保护入口；
- System Settings 是全局共享配置入口，只允许 SUPER_ADMIN 使用，包含本地存储配额、磁盘安全线和清理预览等会影响所有账号的设置；
- 账号会话入口放在后台右上角，后续账号管理页面仍然遵守 SUPER_ADMIN-only 边界。

### Recording 详情

至少显示：

- Recording/Session/File；
- Local 状态；
- COS 状态；
- Bilibili Publication；
- Songs；
- Jobs；
- Protect Local；
- 下载入口。

第一版实时状态使用 polling，不要求 WebSocket。

---

## 15. API 边界

```text
/api/v1/*             Admin API
/api/public/v1/*      Public API
/internal/v1/*        Internal/Recorder API
```

Public DTO 与 Admin DTO 独立。

动作采用明确 Endpoint：

```text
POST /recordings/{id}/actions/protect-local
POST /recordings/{id}/actions/unprotect-local
POST /recordings/{id}/actions/delete-local
POST /jobs/{id}/actions/retry
```

前端不得直接 PATCH 业务状态枚举。

---

## 16. 数据库与文件边界

SQLite 保存：

```text
Users / Policies
Profiles / Runtime
Recordings / Sessions / File metadata
Publishing/COS metadata
Songs
Credentials encrypted metadata
Jobs
Sessions
Audit
Storage Settings
```

SQLite 不保存：

```text
视频二进制
音频二进制
弹幕全文
COS 文件内容
FFmpeg/ASR 临时文件
完整应用日志
```

---

## 17. CI/CD、部署与运维要求

### 分支

```text
dev   → 开发/集成分支，只跑 CI
main  → 正式分支，CI 通过后自动部署生产
```

第一版不建设独立 test/staging 环境。

### GitHub Actions

- Pull Request：CI；
- push `dev`：CI；
- push `main`：CI + Production Deploy；
- Production Deploy 只能由 `main` 触发；
- 使用 SSH/SCP 连接正式服务器；
- SSH 必须使用固定 known_hosts；
- 正式构建在 GitHub Runner 完成，不占用 2C4G 服务器做 Go/Node 编译。

### 部署隔离

普通 7GRecorder 发布：

```text
更新 Backend
更新 Frontend
```

不得无故重启 BililiveRecorder。

BililiveRecorder 使用持久配置/workdir，Backend 短暂部署或故障时应继续独立录制。

### Release

- 每个 Release 以 Git SHA 标识；
- Release 文件有 checksum；
- 服务器保留少量最近 Release 以便人工回滚；
- 不使用 production `latest` tag 自动漂移依赖版本；
- DB migration 前自动备份 SQLite；
- migration 默认保持与上一版应用兼容，避免普通回滚需要 down migration。

### Secret

- GitHub 只保存 SSH 部署凭证；
- 应用 master key/Bilibili/COS/网易云业务 Secret 不通过每次部署下发；
- master key 在服务器独立保存并做离线备份。

### 日志与备份

- 应用日志 stdout/stderr + Docker rotation；
- 不引入 ELK/Loki/Prometheus；
- SQLite 每日安全备份并保留有限历史；
- 临时目录、旧 Release、旧 Docker image 都必须有清理策略。

### 大文件下载

- Local Recording 下载不得由 Go 进程直接转发数 GB 文件，使用鉴权后 Nginx internal/X-Accel-Redirect；
- COS 下载使用短期签名 URL；
- 数据库只保存相对媒体路径，任何删除/下载必须校验不能逃逸数据根目录。

---

## 18. 第一版非目标

```text
高并发 SaaS
微服务
Redis / RabbitMQ / Kafka
PostgreSQL / MySQL
Kubernetes
Workflow Engine / Job DAG
实时视频转码
自研 Bilibili 抓流
实时弹幕二次采集
WebSocket 基础设施
跨模块强依赖流水线
```
