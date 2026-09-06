# 7GRecorder — SQLite 数据库设计

## 1. 定位

`DATABASE.md` 是当前目标数据库结构的权威说明。

SQLite 只保存管理网站、状态和业务元数据，不保存视频、音频、弹幕全文或完整应用日志。

生产数据库：

```text
/data/7grecorder/db/7grecorder.db
```

基础设置：

```text
WAL
foreign_keys = ON
busy_timeout
```

第一版 Internal ID 使用：

```text
INTEGER PRIMARY KEY
```

时间统一保存 UTC，API 使用 RFC3339。

---

## 2. Schema 原则

- Manager → RecordingProfile 从 schema 层按 1:N；第一版应用层限制最多 1 个。
- `recording_profile_id` 是 Recording/Song/Publication/Storage 等主要业务归属字段。
- Recording、Local Storage、Bilibili、COS、Songs 的状态**分别保存**，不使用一个全局 pipeline status。
- 不设计 Job Dependency/DAG 表。
- Job/Event 使用稳定业务键做幂等。
- Secret 加密保存；Password 与 Web Session token 只保存不可逆 hash/digest。
- JSON 只用于平台差异配置、历史 snapshot 或 Job payload，不代替核心关系。
- Profile/历史业务对象优先软归档，不级联物理删除历史 Recording。
- 媒体路径只保存**相对路径**，绝不保存宿主机绝对路径。

### 2.1 SQLite 类型约定

Migration 按以下约定落地：

```text
ID / FK / size / duration / counters    INTEGER
bool                                    INTEGER NOT NULL DEFAULT 0/1
enum / username / path / key            TEXT
timestamp                               DATETIME（UTC）
JSON snapshot/settings                  TEXT
encrypted secret / hash                 TEXT or BLOB, implementation chooses one format consistently
```

规则：

- `created_at/updated_at` 默认 UTC；
- 核心必填字段 `NOT NULL`；
- enum 主要由 Application 校验，不为每个 enum 建难扩展的数据库 CHECK；
- FK 使用 `foreign_keys=ON`；
- 有历史引用的核心表默认 `ON DELETE RESTRICT`；
- `recording_profiles(platform, room_id)` 使用 `WHERE archived_at IS NULL` 的 partial unique index；
- Goose 自己维护 migration version，不创建第二套业务 migration 表。

---

## 3. 用户与权限

### users

```text
id
username                 UNIQUE
password_hash
role                     SUPER_ADMIN | MANAGER
enabled
created_at
updated_at
```

### manager_policies

```text
user_id                  PK/FK
can_edit_recording_profile
can_edit_bilibili_module
can_edit_cos_module
can_edit_netease_module
can_manage_local_files
updated_at
```

---

## 4. Recording Profile

### recording_profiles

```text
id
owner_user_id
name
platform                 default: bilibili
room_id
streamer_name
streamer_uid nullable
timezone                  default: Asia/Shanghai
enabled
public_enabled
public_slug nullable
archived_at nullable
created_at
updated_at
```

约束：

```text
(platform, room_id) 在 active profile 中唯一
public_slug 在非 NULL 时唯一
```

`owner_user_id` 不设 UNIQUE。

### recording_settings

```text
recording_profile_id      PK/FK
auto_record
quality
record_danmaku
segment_duration_sec
finalize_grace_period_sec
updated_at
```

### recording_profile_runtime

```text
recording_profile_id      PK/FK
stream_status             UNKNOWN | OFFLINE | LIVE
recorder_status           UNKNOWN | IDLE | RECORDING | ERROR
sync_status               SYNCED | PENDING | ERROR
current_recording_id nullable
last_event_at nullable
last_reconciled_at nullable
last_error nullable
updated_at
```

---

## 5. Credentials 与可选模块配置

### credentials

```text
id
owner_user_id nullable
scope                     USER | SYSTEM
platform                  bilibili | tencent_cos | netease | ...
purpose                   PUBLISHER | RECORDER_VIEWER | STORAGE | ...
account_label
external_uid nullable
encrypted_secret
secret_format_version      default: 1
status
last_verified_at nullable
created_at
updated_at
```

规则：

- USER Credential 只能被同一个 Manager 拥有的 Profile 引用；
- SYSTEM Credential 只由 SUPER_ADMIN 管理；
- API 永远不返回 `encrypted_secret` 对应明文。

### publishing_profiles

用于 Bilibili、网易云等“平台发布”。

```text
id
recording_profile_id
platform                  bilibili | netease | ...
credential_id nullable
enabled
settings_json
created_at
updated_at
```

建议一个 Profile 同一 platform 只保留一个 active publishing profile。

### cos_storage_profiles

COS 是 Storage，不和 Bilibili Publication 混表。

```text
id
recording_profile_id
credential_id
enabled
region
bucket
prefix
max_managed_bytes
created_at
updated_at
```

第一版一个 Recording Profile 最多一个 active COS profile。

---

## 6. 系统级本地存储配置

### local_storage_settings

第一版单服务器只有一行。

```text
id                       fixed single row
max_recording_bytes
min_system_free_bytes
cleanup_target_ratio
absolute_emergency_free_bytes
updated_at
updated_by_user_id
```

说明：

- `max_recording_bytes`：所有 Profile 原始录播合计最大预算；
- `min_system_free_bytes`：给 OS/其他服务预留；
- `cleanup_target_ratio`：触发滚动清理后清到该比例；
- `absolute_emergency_free_bytes`：真正硬安全线。

---

## 7. Recording

### recordings

```text
id
recording_profile_id
title nullable
started_at
ended_at nullable
completed_at nullable
finalize_at nullable
duration_ms nullable
recording_status          ACTIVE | FINALIZING | COMPLETED | ABORTED
song_processing_status    DISABLED | PENDING | PROCESSING | REVIEW_REQUIRED | COMPLETED | SKIPPED | FAILED | SOURCE_MISSING
local_storage_status      AVAILABLE | PARTIAL | DELETED
local_protected           default false
local_deleted_at nullable
source_room_id
streamer_name_snapshot
recording_config_snapshot_json nullable
created_at
updated_at
```

注意：

- Bilibili/COS 状态不放在 `recordings` 大状态机里；
- 本地滚动删除不依赖 Publication/COS/Songs 成功；
- `local_storage_status` 只是聚合显示，可由 recording_files 计算/维护。

### recorder_sessions

```text
id
recording_id
external_session_id nullable
started_at
ended_at nullable
created_at
updated_at
```

### recording_files

```text
id
recording_id
recorder_session_id nullable
relative_path       UNIQUE
original_name
kind                video | danmaku | metadata
file_status         WRITING | CLOSED | MISSING | DELETED
segment_index nullable
size_bytes nullable
duration_ms nullable
checksum nullable
opened_at nullable
closed_at nullable
deleted_at nullable
created_at
updated_at
```

本地滚动存储删除后保留 metadata，只把：

```text
file_status = DELETED
relative_path metadata 保留
```

用于历史审计和 UI。

Early production bootstrap may estimate `recordings.duration_ms` and `recording_files.duration_ms` from the
recording filename timestamp plus the local file mtime. This is UI metadata for spotting unusually short
segments; precise media duration can replace it later through an FFmpeg/ffprobe metadata scan.

### recorder_events

```text
id
event_id            UNIQUE
event_type
room_id
recording_profile_id nullable
payload_json
event_at
processed_at nullable
created_at
```

仅用于业务幂等/恢复，按保留策略清理。

---

## 8. Bilibili / External Publication

### publications

```text
id
recording_profile_id
recording_id nullable
song_id nullable
platform
credential_id nullable
external_id nullable
external_url nullable
status              PENDING | UPLOADING | VERIFYING | VERIFIED | FAILED | AMBIGUOUS | SOURCE_MISSING
attempts
last_error nullable
request_snapshot_json nullable
published_at nullable
verified_at nullable
created_at
updated_at
```

约束建议：

```text
(recording_id, platform) 对完整录播类 Publication 唯一
```

Bilibili Publication 与 COS 副本是不同概念。

---

## 9. COS 对象

### cos_objects

每一条表示 7GRecorder 管理的一份 COS 文件副本。

```text
id
cos_storage_profile_id
recording_profile_id
recording_id
recording_file_id
object_key
size_bytes
checksum nullable
etag nullable
status               PENDING | UPLOADING | AVAILABLE | FAILED | DELETED | SOURCE_MISSING
last_error nullable
uploaded_at nullable
deleted_at nullable
created_at
updated_at
```

约束：

```text
(cos_storage_profile_id, recording_file_id) UNIQUE
object_key UNIQUE within managed profile/prefix
```

COS Rolling Usage 只统计：

```text
status = AVAILABLE
```

且只删除数据库登记、属于该 Storage Profile Prefix 的对象。

---

## 10. Songs

### songs

```text
id
recording_profile_id
recording_id
title nullable
artist nullable
start_ms
end_ms
confidence nullable
status
local_audio_status        NONE | AVAILABLE | DELETED
audio_relative_path nullable
created_at
updated_at
```

### song_candidates

```text
id
song_id
title nullable
artist nullable
source
score
evidence_json nullable
created_at
```

---

## 11. Jobs

### jobs

```text
id
recording_profile_id nullable
recording_id nullable
recording_file_id nullable
song_id nullable
publication_id nullable
cos_object_id nullable

type
resource_class              LIGHT | NETWORK | MEDIA | AI | MAINTENANCE
business_key                UNIQUE where applicable
payload_json nullable
status                      PENDING | RUNNING | SUCCEEDED | FAILED | CANCELLED
priority
attempts
max_attempts
run_after
locked_at nullable
heartbeat_at nullable
locked_by nullable
last_error_class nullable       TRANSIENT | AUTH | SOURCE_MISSING | PERMANENT | AMBIGUOUS
last_error nullable
created_at
updated_at
```

显式关联列用于：

- UI 查询；
- 本地 Cleanup 判断文件/Recording 是否正在被任务读取；
- 避免依赖解析 JSON payload。

第一版不增加：

```text
job_dependencies
workflow_id
workflow_nodes
```

### 典型 business_key

```text
profile:5:recorder:sync
recording:123:bilibili:upload
publication:45:bilibili:verify
file:456:cos:upload
song:987:cut
recording:123:local-cleanup
```

---

## 12. Web Session / Audit / Settings

### sessions

```text
id
user_id
token_digest             UNIQUE
expires_at
last_seen_at
created_at
```

### audit_logs

```text
id
actor_user_id
action
resource_type
resource_id
summary
created_at
```

只保存必要管理操作，不作为应用日志系统。

### system_settings

只用于少量不值得独立成表的低风险配置；核心存储/模块配置优先独立表。

---

## 13. 关键 FK / 唯一约束

Migration 至少落实：

```text
manager_policies.user_id
  → users.id UNIQUE

recording_profiles.owner_user_id
  → users.id

recording_settings.recording_profile_id
  → recording_profiles.id UNIQUE

recording_profile_runtime.recording_profile_id
  → recording_profiles.id UNIQUE

publishing_profiles.recording_profile_id
  → recording_profiles.id

publishing_profiles.credential_id
  → credentials.id

cos_storage_profiles.recording_profile_id
  → recording_profiles.id
  UNIQUE in v1

recordings.recording_profile_id
  → recording_profiles.id

recorder_sessions.recording_id
  → recordings.id

recording_files.recording_id
  → recordings.id

recording_files.recorder_session_id
  → recorder_sessions.id nullable

publications.recording_profile_id
  → recording_profiles.id

publications.recording_id/song_id
  nullable FK

cos_objects.recording_file_id
  → recording_files.id

songs.recording_id
  → recordings.id

jobs.*_id
  nullable FK to corresponding business entity

sessions.user_id
  → users.id
```

业务唯一性：

```text
active recording profile: (platform, room_id)
public_slug when non-null
publishing profile: (recording_profile_id, platform)
cos storage profile: recording_profile_id
recording file: relative_path
recorder event: event_id
full-recording publication: (recording_id, platform)
COS copy: (cos_storage_profile_id, recording_file_id)
Job: business_key when non-null
session: token_digest
```

不使用数据库级跨模块 trigger 自动推进业务状态。

---

## 14. 推荐索引

保持最少必要索引：

```text
recordings(recording_profile_id, started_at)
recordings(recording_status, finalize_at)
recordings(completed_at)

recording_files(recording_id)
recording_files(file_status, closed_at)
recording_files(relative_path UNIQUE)

recorder_events(event_id UNIQUE)

publications(recording_profile_id, platform, status)
publications(recording_id, platform)

cos_objects(cos_storage_profile_id, status, uploaded_at)
cos_objects(recording_id)

songs(recording_id, start_ms)

jobs(status, run_after, priority)
jobs(recording_id, status)
jobs(recording_file_id, status)
jobs(business_key UNIQUE)

sessions(token_digest UNIQUE)
```

不为假设中的未来性能预建大量索引。

---

## 15. 明确不入库的数据

```text
FLV / MP4 binary
M4A / WAV binary
弹幕全文
缩略图 binary
完整应用日志
FFmpeg 临时文件
Whisper 临时文件
COS object binary
```

数据库只保存 relative_path/key/metadata/status。

所有相对路径必须在 Application/Filesystem Adapter 中 resolve 并校验仍位于受控 root 内；数据库记录本身不得被直接当成可删除的宿主机绝对路径。

---

## 16. SQLite Backup

Recording groups are currently computed from existing `recordings` and `recording_files` rows. No dedicated group table
is introduced for the read-only diagnostics step. If a later FFmpeg merge job creates durable upload assets, the schema
must be updated first to record derived source metadata and source recording IDs.

备份写入：

```text
/data/7grecorder/backups/db/
```

使用 SQLite 安全备份方式；默认保留 7–30 份。

数据库备份本身不包含媒体文件。
