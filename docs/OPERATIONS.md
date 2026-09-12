# 7GRecorder — 运行配置、运维与恢复设计

## 1. 目标

单机项目最重要的运维目标不是复杂监控体系，而是：

1. 录播不能把系统盘写爆；
2. Backend 故障/部署不能连带停止 BililiveRecorder；
3. SQLite/Secret 可恢复；
4. 外部模块失败能够在管理后台直接看见；
5. 日志、Docker image、temp 不应长期无限增长。

---

## 2. 配置分层

### 静态运行配置

不需要在管理后台频繁修改，来自：

```text
/etc/7grecorder/app.env
```

典型内容：

```text
APP_LISTEN_ADDR
APP_PUBLIC_BASE_URL
DATA_ROOT
SQLITE_PATH
TEMP_ROOT
RECORDER_BASE_URL
RECORDER_BASIC_USER
RECORDER_BASIC_PASSWORD
MASTER_KEY_PATH
LOG_LEVEL
```

这些配置：

- 启动时读取；
- 不进 SQLite；
- 不通过 Web API 修改；
- 修改后允许重启 Backend 生效。

### 动态业务配置

在管理后台修改并存 SQLite：

```text
Users / Policies
Recording Profiles
Recording Settings
Bilibili Publishing
COS Storage
Songs
Local Storage Quota
Credentials encrypted payload
```

---

## 3. Secret

### Master Key

推荐：

```text
/etc/7grecorder/master.key
```

要求：

- 随机生成；
- mode 0600；
- 不进 Git；
- 不进 SQLite；
- 不输出日志；
- 必须单独做离线备份。

数据库中的 Bilibili/COS/网易云 Secret 使用 master key 加密。

**只备份 SQLite 而不备份 master key，会导致恢复后无法解密外部平台 Credential。**

### Password

管理网站用户密码只保存：

```text
password_hash
```

不做可逆加密。

---

## 4. 首次管理员与运维 CLI

不内置默认管理员密码。

空数据库首次部署后，通过服务器命令创建 SUPER_ADMIN，例如设计命令：

```text
7grecorder admin bootstrap
```

使用明确的一次性安全参数；成功后如果已有 SUPER_ADMIN，再次 bootstrap 默认拒绝。

```bash
docker compose --env-file /etc/7grecorder/app.env run --rm --no-deps 7grecorder admin bootstrap \
  --username admin \
  --password 'replace-with-a-long-random-password'
```

重置已有账号密码：

```bash
cd /opt/7grecorder/deploy
GIT_SHA="$(cat /opt/7grecorder/current-release)" \
docker compose --env-file /etc/7grecorder/app.env run --rm --no-deps 7grecorder admin reset-password \
  --username admin \
  --password 'replace-with-a-new-long-random-password'
```

建议同一个 binary 最终提供少量运维命令：

```text
7grecorder migrate
7grecorder db backup
7grecorder admin bootstrap
7grecorder admin reset-password
7grecorder doctor
```

这些命令复用 Application/Platform 能力，不创建第二套运维程序。

如果 SUPER_ADMIN 忘记密码，可通过服务器 CLI reset，不需要数据库手改 hash。

---

## 5. 外部组件凭证

### BililiveRecorder HTTP

Recorder API 仅在内部网络使用，但仍建议配置 HTTP Basic。

凭证属于：

```text
static integration secret
```

存在 `/etc/7grecorder/recorder.env` / Compose secret-style env 中，而不是普通 Manager Credential。

### Bilibili/COS/NetEase

属于业务 Credential：

```text
encrypted_secret in SQLite
```

Admin API 只写不读。

---

## 6. 数据目录

建议：

```text
/data/7grecorder/
├── db/
│   └── 7grecorder.db
├── recordings/
├── songs/
├── temp/
└── backups/
    └── db/
```

数据库保存媒体文件**相对路径**，不保存宿主机绝对路径。

运行时通过配置的 root resolve：

```text
relative path
+ configured root
→ absolute runtime path
```

所有 read/delete/download 操作必须校验：

- clean path；
- 不包含 `..` 越界；
- resolve 后仍在允许 root 内；
- symlink/realpath 不可逃逸到 root 外。

---

## 7. Recording 输出目录

BililiveRecorder 的 Room 配置由 7GRecorder 设置成 Profile 可识别的稳定目录。

原则：

```text
recordings/<profile-id>/...
```

文件名包含足够的时间/room 信息，便于：

- Webhook 路由；
- Backend restart 后扫描恢复；
- 人工 SSH 排查。

第一版不要求文件名直接包含 7GRecorder Recording ID，因为 Recorder 在开始写文件时未必已获得业务 Recording ID。

Recording/File 归属最终以 SQLite + room/profile + event/reconciliation 为准。

---

## 8. Local Storage Guard

系统级动态配置：

```text
max_recording_bytes
min_system_free_bytes
cleanup_target_ratio
absolute_emergency_free_bytes
```

快速 usage 以 SQLite 中 CLOSED/AVAILABLE file 的 `size_bytes` 汇总；系统真实剩余空间使用 filesystem/statfs。周期 reconciliation 再用实际文件校正 DB，避免每 30 秒全盘递归扫描。

行为：

### Normal

继续录制和普通 Job。

### Quota/Low Free Warning

Local Storage Reconciler 以**整场 Recording** 为候选单位：

```text
oldest COMPLETED recording
→ if any file is protected / writing / RUNNING-job-in-use: skip whole Recording
→ delete all eligible local files of that Recording
→ continue until target reached
```

普通滚动清理不主动制造“只剩半场”的 Recording。

不检查 Bilibili/COS/Songs 是否成功。

### Emergency

如果已经执行可安全清理后仍低于硬安全线：

- 不删除 protected/active/writing/in-use 文件；
- 阻止新的 NETWORK/MEDIA/AI 重任务；
- 可暂停开始新的自动录制，以保护 OS 和同机其他服务；
- Dashboard 显示 CRITICAL；
- 已经正在写入的 Recording 不做粗暴 `rm`。

这是资源安全边界，不是跨业务模块依赖。

---

## 9. COS Managed Quota

COS 使用 7GRecorder 自己的逻辑预算：

```text
max_managed_bytes
```

只统计数据库中：

```text
status=AVAILABLE
```

且属于该 COS Storage Profile 的对象。

超过预算：

```text
按最旧 Recording
→ 删除该 Recording 在此 COS Profile 下的 managed objects
→ 直到回到预算
```

永远不：

- 根据 Bucket 全量对象自行猜测归属；
- 删除数据库没有登记的 object；
- 删除配置 Prefix 之外对象。

如果 COS 服务返回 quota/storage 类错误，可先执行 managed cleanup 后再按 retry policy 重试，但仍只操作自己的对象。

---

## 10. Temp

每个 Job：

```text
temp/<profile-id>/<job-id>/
```

Job 成功/失败结束后尽量立即删除。

Scheduler 定期清理：

- 已不存在 RUNNING Job；
- 超过安全时间；
- 位于 temp root 内；

的遗留目录。

禁止使用系统随机目录存长期业务结果。

---

## 10.5 Disk Housekeeping

Disk housekeeping 是独立的运维清理，不替代 Local Storage Guard，也不改变 Recording/Bilibili/COS/Songs
业务状态。

运行时机：

```text
每次部署前
每次部署成功后
每天定时一次
磁盘 CRITICAL 时可由 SUPER_ADMIN 手动触发
```

默认安全白名单：

- `/opt/7grecorder/deploy/7grecorder-release-*.tar`：保留最近少量，其余删除；
- `/opt/7grecorder/releases/<sha>`：保留 current 和最近少量 release，其余删除；
- Docker：删除未被容器引用的旧 `7grecorder:<sha>` image、dangling image、24 小时以前的 build cache；
- `/data/7grecorder/temp`：删除不属于 RUNNING Job 且超过安全时间的遗留目录；
- `/data/7grecorder/backups/db`：保留最近 14 份或配置数量，删除更旧备份；
- apt package cache：允许 `apt-get clean`；
- systemd journal：允许按大小/时间 vacuum，例如保留最近 7–14 天或固定上限；
- Docker container log：优先通过 log rotation 限制，housekeeping 只检查/告警，不直接 truncate 活跃日志文件。

禁止项：

- 不直接删除 `/data/7grecorder/recordings`；原始录播只能通过 Local Storage Guard 的 completed/unprotected/not
  in-use 条件删除；
- 不直接删除 `/data/7grecorder/upload-sources` 中仍可能被 Bilibili/COS/Songs 读取的文件；
- 不删除 BililiveRecorder 当前 image、workdir 或配置；
- 不删除 SQLite WAL/SHM 文件来“清理空间”；
- 不扫描 COS Bucket 删除数据库未登记对象。

Worker 已实现磁盘压力自动回收：只有 Upload Source 的全部 enabled 远端模块确认成功、审核已结束、没有运行中任务，
并且该来源不是对应录制配置最新一场时，才删除其已关闭原始视频和 `DATA_ROOT/upload-sources/<profile>/<source>`
受控派生目录。数据库历史、弹幕、Publication/COS 元数据和远端下载信息继续保留。

---

## 11. Logging

应用：

```text
structured stdout/stderr
```

由 Docker logging driver 管理。

第一版不引入：

```text
ELK
Loki
Prometheus
```

Docker 日志必须配置 rotation，例如：

```text
max-size
max-file
```

避免 JSON log 无限增长。

SQLite 只保存：

- audit log；
- Job last_error/error_class；
- 关键业务状态。

不保存完整程序日志。

---


### 11.1 轻量数据保留

为了避免数据库和日志长期无界增长，第一版默认：

```text
recorder_events        30 days
SUCCEEDED jobs         30 days
CANCELLED jobs         30 days
FAILED jobs            90 days
expired web sessions   daily cleanup
audit_logs             180 days
DB backups             14 copies
Docker logs            rotation by size/count
temp leftovers         cleanup after safe age
deploy artifacts       keep current + recent releases
Docker build cache     daily/deploy cleanup after safe age
```

这些是默认值，不要求第一版全部做成 Admin 可配置项。

Recorder/File/Publication/Song 等业务历史 metadata 不因为上述 housekeeping 自动删除。

SQLite 不定期自动 `VACUUM`，避免大文件临时空间和锁；优先依赖正常 page reuse、WAL checkpoint/SQLite optimize，确有需要再人工维护。

## 12. Health

### Machine/System

Admin Dashboard 显示：

- system disk free；
- local recording usage/quota；
- temp usage；
- DB size；
- current release SHA；
- Job backlog；
- running Jobs。

### Modules

独立显示：

```text
BililiveRecorder API
Bilibili Credential
COS Credential/Bucket
NetEase Credential
FFmpeg
biliup
optional AI tool
```

某模块 unhealthy 不影响整个 Backend readiness。

---

## 13. Reconciliation

Module autonomy 依赖 Reconciler 兜底。

推荐第一版默认频率（可作为静态配置调整）：

```text
Recording finalizer       10–15s
Recorder reconciliation  30–60s
Local storage guard       30s
Bilibili reconcile        30–60s
COS reconcile             30–60s
Songs reconcile           60s
stale Job recovery        30s
temp cleanup              10–30min
DB backup                 daily
```

同时，新 Event/Job 可通过进程内 wakeup signal 立即唤醒 Worker；SQLite 仍是唯一 durable source of truth。

---

## 14. Recorder Reconciliation

Webhook 是主事件通道，但不能是唯一恢复机制。

Backend：

- 启动时；
- 周期性；

检查 BililiveRecorder：

```text
configured rooms
runtime status
7GRecorder desired profiles
```

并修正：

- 缺失 Room；
- 多余/归档 Room；
- AutoRecord/config 漂移；
- runtime 状态。

文件层面：

- 对 DB 已知 file 验证是否仍存在；
- 对 Recorder 输出目录发现的、符合受控命名/目录规则的未登记文件，允许恢复登记；
- 无法安全确定归属的文件只报告为 orphan，不自动删除。

---

## 15. Subprocess

`biliup`、FFmpeg、AI 等使用 `exec.CommandContext` 类机制。

要求：

- Job 独立 temp directory；
- stdout/stderr 有大小限制或摘要，不无限写 SQLite；
- cancellation/timeout 能终止子进程；
- Job 完成后回收；
- 不用 shell 拼接用户输入；
- 文件参数来自验证后的内部路径。

直播时默认不启动新的 NETWORK/MEDIA/AI Job。

---

## 16. Error 分类

Adapter/Application 使用少量稳定错误类别：

```text
TRANSIENT
AUTH
SOURCE_MISSING
PERMANENT
AMBIGUOUS
```

含义：

### TRANSIENT

网络 timeout、临时 5xx，可 retry。

### AUTH

Cookie/key 失效。停止无意义重试，等待 Credential 修复。

### SOURCE_MISSING

本地源已滚动删除。

### PERMANENT

配置/格式/平台明确拒绝等不会靠重试解决的问题。

### AMBIGUOUS

外部副作用可能已发生，例如上传成功但本地进程在保存 external_id 前崩溃。

Job 可把 `error_class + last_error` 保存到 SQLite，UI 按类别指导管理员。

### 默认 Retry Policy

Worker execution is resource-class concurrent: one `LIGHT` loop, one `MEDIA` loop, and two `NETWORK` loops. This allows
one Bilibili upload and one COS upload to progress at the same time while keeping FFmpeg media work serialized. Long
uploads refresh job progress and `heartbeat_at`; `COS_UPLOAD_MAX_BYTES_PER_SECOND` can cap COS request-body throughput
per COS worker. Bilibili throughput is controlled by the persisted biliup `upload_limit` concurrency setting until a
separate network shaper is designed.

The admin Jobs page is the authoritative live progress view. Bilibili shows aggregate transferred bytes across all
parts after the adapter parses biliup progress output. COS shows per-object network upload progress after its optional
compression stage; while FFmpeg is still producing the derived object, the recording detail shows `COMPRESSING` and no
network percentage is expected. `COS_COMPRESSION_THREADS` defaults to 2 so this stage does not consume every CPU core.

不是全局统一指数退避，Job Handler 使用简单默认值：

| Job | 自动尝试 | 建议退避 | 特殊规则 |
| --- | ---: | --- | --- |
| SYNC_RECORDER_PROFILE | 5 | 5s / 30s / 1m / 5m / 15m | Recorder unavailable 属 TRANSIENT |
| MERGE_UPLOAD_SOURCE | 3 | 5s / 30s / 1m | terminal 后显示 MERGE_FAILED |
| PACKAGE_UPLOAD_SOURCE | 3 | 5s / 30s / 1m | terminal 后显示 PACKAGE_FAILED |
| UPLOAD_BILIBILI | 3 | 1m / 5m / 15m | stale/crash 后先 AMBIGUOUS，不盲重传 |
| VERIFY_BILIBILI | 8–10 | 1m → 1h 逐步拉长 | 平台处理延迟允许较长验证窗口 |
| UPLOAD_COS_OBJECT | 5 | 30s / 2m / 5m / 15m / 30m | AUTH/SOURCE_MISSING 不 retry |
| UPLOAD_COS_RECORDING_FILE | 5 | 30s / 2m / 5m / 15m / 30m | raw danmaku archive; AUTH/SOURCE_MISSING 不 retry |
| CUT_SONG_AUDIO | 2 | 1m | 同一输入重复失败通常需要人工看源文件 |
| AI jobs | 2 | 5m | 最低优先级 |
| CLEANUP | 3 | 1m / 5m / 15m | 删除前每次重新做安全条件检查 |

通用错误处理：

```text
TRANSIENT       → 根据 Job policy retry
AUTH            → stop retry，等待 Credential 修复
SOURCE_MISSING  → stop retry
PERMANENT       → stop retry
AMBIGUOUS       → module-specific verify/reconcile/manual decision
```

手动 Retry 复用原 Job/business_key，不创建第二个业务 Job；实现可为原 Job 增加一次新的允许尝试并立即 `run_after=now`。

RUNNING Job 的取消：

- 本地可逆任务可以终止 subprocess；
- 对 Bilibili/外部 Publisher 等可能已产生副作用的任务，取消/进程失联后按 `AMBIGUOUS` 处理。

---

## 17. SQLite Backup

默认每日做 online backup：

```text
/data/7grecorder/backups/db/
```

建议默认保留：

```text
14 份
```

由于 DB 很小，空间成本低。

备份要求：

- 使用 SQLite 安全 backup API/机制；
- 不直接复制正在活跃 WAL 状态下无法保证一致性的文件组合；
- backup 完成后校验可以打开；
- Deployment migration 前额外创建一份 pre-deploy backup。

媒体文件不属于 DB backup。

SQLite WAL 需要正常 checkpoint 策略，避免 `.wal` 长期异常增长；Backup/Deploy 前可做安全 checkpoint，但不能通过粗暴删除 WAL/SHM 文件“清理空间”。

---

## 18. 恢复

### Backend Container 崩溃

BililiveRecorder 继续。

Backend 重启：

```text
migrate check
→ stale job recovery
→ recorder reconciliation
→ module reconciliation
```

### SQLite 损坏/误操作

```text
stop backend
→ preserve damaged DB
→ restore latest valid DB backup
→ start
→ recorder/filesystem reconciliation
```

恢复后可能存在“文件已写但 DB backup 尚未记录”的窗口，Reconciler 尽量重新发现。

### Master Key 丢失

不能从 DB 恢复。

需要：

- 使用离线备份恢复 master key；
- 或重新配置所有外部 Credential。

因此 master key backup 是上线前必做运维事项。

---

## 19. 依赖版本

外部依赖版本固定在仓库/构建定义：

- BililiveRecorder；
- biliup；
- FFmpeg；
- Tencent COS Go SDK；
- GoFrame；
- Go；
- Node/pnpm；
- frontend packages。

不在生产服务器运行“自动升级到最新版”。

每次升级 Recorder/biliup 需更新 Adapter fixture/test。

COS 第一版使用腾讯云官方 Go SDK：

```text
github.com/tencentyun/cos-go-sdk-v5
```

它是应用依赖，不是额外常驻中间件。

---

## 20. 上线前最低运维检查

```text
[ ] /data/7grecorder 容量和权限正确
[ ] local quota < 磁盘总空间，且预留 OS/其他服务空间
[ ] Docker log rotation
[ ] master.key 已离线备份
[ ] SQLite backup 可恢复
[ ] BililiveRecorder API 仅内网/localhost
[ ] Nginx /internal 未公网暴露
[ ] SSH known_hosts pinned
[ ] GitHub deploy key 正确
[ ] 当前 Recorder/biliup/FFmpeg 版本已固定
[ ] backend deploy 不重启 recorder
```

---

## 21. Upload Review Runbook

Use the admin recording page for normal review operations. Do not edit SQLite directly and do not retry Bilibili/COS
jobs while the source is waiting for review.

1. Click `Require review` as early as possible. The source should show `REQUIRED`; Bilibili and COS should show waiting
   for review. Local merge/package may continue.
2. Expand details and download the current publish parts for inspection.
3. Enter deletion ranges on the complete parent timeline, one `HH:MM:SS-HH:MM:SS` range per line, then click
   `Apply edit` once.
4. Wait for `APPLY_UPLOAD_SOURCE_EDIT` to succeed. Confirm paths are `edited/...` and verify the displayed duration and
   size. Remaining in `REQUIRED`/waiting-for-review state is expected.
5. Download edited parts when content verification is needed. Only then click `Approve review`.
6. Confirm Bilibili and COS move independently through pending/uploading/verified or available states.

Read-only diagnosis:

```sql
SELECT id, status, review_status, edit_decision_json, duration_ms, total_bytes, last_error
FROM upload_sources ORDER BY id DESC;

SELECT id, type, status, attempts, upload_source_id, locked_at, heartbeat_at, last_error
FROM jobs WHERE upload_source_id = ? ORDER BY id;

SELECT id, sort_order, relative_path, duration_ms, size_bytes, status
FROM upload_source_outputs WHERE upload_source_id = ? ORDER BY sort_order;
```

Interpretation:

- `review_status=REQUIRED` plus empty `edit_decision_json` plus `edited/...` outputs means editing completed and is
  waiting for human approval.
- Non-empty `edit_decision_json` means edit work is still pending/running or failed; do not approve.
- A failed edit job leaves the review gate in place. Inspect `jobs.last_error`, fix the cause, then use the supported
  retry action; do not point output rows at partial files.
- If review was requested near the end of a Bilibili submission, cancellation is externally ambiguous. Check the
  Bilibili creator center before retrying to avoid duplicate投稿.
- Missing edited/local output means do not approve. Repair/retry media work while retaining the review gate.

Only 7GRecorder needs to be stopped for an emergency upload freeze; BililiveRecorder should remain running. Stopping
the service is not the normal review mechanism and does not replace the persisted review state.

## 22. Automatic Disk-pressure Cleanup

The worker checks local storage pressure during its periodic reconciliation loop. It reclaims oldest eligible upload
sources until the configured target is met, while retaining the newest source for every recording profile.

Eligibility requires all enabled remote modules to be confirmed successful. Consequently, a source with Bilibili or
COS `FAILED`, `PENDING`, or `UPLOADING`, a source waiting for review, and the current active recording are never deleted.
The cleanup keeps SQLite history and remote metadata; only local closed source videos and the source-owned derived
directory are removed.

Read-only verification:

```sql
SELECT id, started_at, status, review_status, local_cleanup_status, local_deleted_at
FROM upload_sources ORDER BY started_at DESC;
```

`DELETING` means the source was durably claimed before filesystem work. `FAILED` means cleanup itself needs inspection;
it must not be repaired or uploaded as if local disappearance were accidental. `DELETED` is the expected terminal
local state for an older remotely archived source.
