# 7GRecorder — 外部工具与平台集成边界

## 1. 原则

外部组件全部通过 Adapter/Port 隔离。

核心业务不得直接依赖：

- BililiveRecorder HTTP DTO；
- biliup CLI stdout 文本；
- FFmpeg 参数；
- Tencent COS SDK 类型；
- 网易云等平台 SDK 类型。

Adapter 负责把外部世界映射成 7GRecorder 内部稳定语义。

---

## 2. BililiveRecorder

### 角色

唯一职责：

```text
直播检测/抓流
录制
分段
弹幕落盘
```

7GRecorder 不重写这些能力。

### 运行模式

使用持久 workdir 的标准 `run` 模式。

不使用 config-less portable 作为生产默认，因为 Backend 独立部署/故障时 Recorder 仍应保留 Room 配置并继续录制。

### 控制方向

```text
7GRecorder Desired State
        │ HTTP API
        ▼
BililiveRecorder
```

使用 HTTP API：

- list/add/remove rooms；
- read/update room config；
- start/stop/split 等必要运维动作；
- runtime/query。

### 事件方向

```text
BililiveRecorder
      │ Webhook v2
      ▼
7GRecorder Event Inbox
```

至少处理：

```text
StreamStarted
StreamEnded
SessionStarted
FileOpening
FileClosed
SessionEnded
```

事件 mapping 必须在 pinned Recorder 版本基础上保存 fixture。

### Desired State

SQLite RecordingProfile 是业务配置来源。

Recorder workdir/config 是**运行保障副本**：

- Backend down 时允许 Recorder 独立继续；
- Backend 恢复后 reconciliation 以 SQLite Desired State 修正漂移。

固定版本 `BililiveRecorder 2.18.0` 的房间设置接口为 `POST /api/room/{roomId}/config`。请求必须使用该版本
`SetRoomConfig` DTO 的 `OptionalRecordMode`、`OptionalCuttingMode`、`OptionalCuttingNumber`、
`OptionalRecordDanmaku`、`OptionalRecordingQuality` 字段，并以 `{ "HasValue": true, "Value": ... }`
表达房间级覆盖值；不能把配置文件中的 `RecordDanmaku` 等字段名直接当作 HTTP DTO。

同步成功必须同时满足 HTTP 200 与响应 DTO 中 AutoRecord、分段、弹幕、画质的有效值和期望值一致。HTTP 成功但
响应缺字段或值不一致视为同步失败，写入 runtime ERROR，避免产生“后台已开启但 Recorder 实际沿用默认值”的假成功。
Backend Worker 每次启动都会把 Recorder 配置同步 Job 重新排队，以修正 Backend 停机期间或旧版本造成的配置漂移。

日常业务不通过 BililiveRecorder WebUI 修改 Room；WebUI 只用于 debug/运维。

---

## 3. biliup

### 角色

只承担 Bilibili 投稿/查询相关执行能力。

第一版使用：

```text
CLI subprocess
```

不运行独立 biliup server。

### Pinned Runtime

The production image installs `biliup==1.2.4` from PyPI and invokes the CLI directly. The adapter is intentionally
small and only builds arguments supported by the pinned CLI:

```text
biliup -u <job-temp-cookies.json> upload --submit <app|web|client> --limit <n> --copyright <1|2> --tid <partition> --title <title> --desc <description> ...
```

The default upload settings are:

- submit mode: `app`
- upload concurrency limit: `1`
- Bilibili partition `tid`: default `2047` (virtual creator). If biliup/Bilibili rejects the partition explicitly, retry
  once with fallback `27` instead of retrying on ambiguous network or submission errors.
- copyright: `1` when the stored config omits or corrupts the value

Credential secret JSON should contain the content of a biliup-generated `cookies.json`, either directly or under
`cookie_file`, `cookie_json`, or `biliup_cookie`. Any valid JSON object is passed through to biliup so minor upstream
cookie-file shape changes do not break the app. Browser cookie strings such as `SESSDATA=...` are rejected because
they are not a stable biliup credential file format.

If the CLI exits successfully but no BV id can be parsed from stdout, 7GRecorder still marks the publication verified
without an external URL. This avoids unsafe duplicate submissions; a later verification/list adapter can fill the URL.

### Upload Source Rule

第一版整场 Recording 投稿要求：

```text
Upload Source = READY_TO_UPLOAD
source file = Local AVAILABLE
```

如果已经因本地滚动存储造成必要分段缺失：

```text
Publication = SOURCE_MISSING
```

第一版不默认“只上传剩余分段形成半场稿件”，也不自动从 COS 回源后再投稿。

### Credential

Worker：

```text
decrypt Credential
→ job temp credential file
→ chmod restricted
→ biliup --user-cookie ...
→ remove temp file
```

不把明文 Cookie 写长期普通文件。

### 输出

Adapter 只向 Application 暴露稳定结果：

```text
UploadSucceeded(external identifiers if known)
UploadFailed(error class)
UploadAmbiguous
Verification result
```

7GRecorder renders Bilibili title, description, tags, and multipart file metadata before calling the adapter. The
adapter receives a stable upload request and must not read plaintext credential files from a long-lived location.

Before invoking biliup, the adapter exposes each source through a per-job temporary alias named `p01.<ext>`,
`p02.<ext>`, and so on. biliup therefore submits concise part titles while the canonical local filenames, persisted
paths, and COS object names remain unchanged. Aliases must not copy multi-GB media and are removed with the credential
work directory after the command exits.

CLI 文本解析集中在 Adapter。

升级 biliup 必须更新 fixture。

Biliup does not expose a hard bytes-per-second upload limit in the pinned CLI. 7GRecorder exposes and persists the
CLI concurrency limit (`upload_limit`, default 1) and streams adapter output to job progress. The adapter normalizes
ANSI/control sequences from the pinned CLI progress renderer before parsing transferred and total bytes; the admin
Jobs page displays the resulting percentage. A true Bilibili bandwidth
cap requires an explicit external shaper/proxy design. COS upload progress is measured in-process by wrapping the SDK
request body and is displayed by the same Jobs progress component once network upload begins.

---

## 4. FFmpeg / ffprobe

### 角色

```text
probe media
extract/cut audio
必要的轻量媒体处理
COS delivery compression
```

第一版不做通用转码平台。

Adapter 输入：

- 受控本地相对路径解析后的文件；
- 时间区间；
- 输出格式。

Adapter 输出：

- duration/metadata；
- 结果相对路径；
- typed error。

不把任意前端参数直接拼接为 shell command。

### COS 发布分片方案

COS Adapter 直接读取当前 `upload_source_outputs` 的原始发布分片，不调用 FFmpeg，也不创建 ZIP、7z 或其他派生
压缩文件。Bilibili 可以同时读取同一源文件，但两个模块各自维护任务、进度和结果。

### Resolution-changing recordings

Before `PackageSegments` uses the concat demuxer with `-c copy`, FFprobe reads a normalized stream signature for each
input. Consecutive inputs are concatenated only while video resolution/codec/profile/level/pixel format/frame rate and
audio codec/sample-rate/channel layout remain compatible. A PK layout or other stream-parameter change starts a new
publish part. The adapter does not scale or stretch either input and does not re-encode the full session merely to
force one file.

The first implementation detects changes at recording-file boundaries. If a single source file itself contains an
undetected mid-file codec reconfiguration, it remains a separate diagnostic case and must not be solved by silently
applying a lossy fallback.

新对象使用 `videos/YYYY-MM-DD/session-NN/pNN.<source-format>`。日期和当天场次来自父 Upload Source，分片序号
来自当前 output manifest。历史 `h264_crf23_medium_mp4` 对象保留其已登记 key、格式和下载能力，不做批量迁移；
服务器环境中遗留的 COS 压缩变量不再控制新对象行为。

### Reviewed media edits

Review cuts are deletion ranges on the complete upload-source timeline. The media adapter normalizes the ranges,
derives the complementary keep ranges, reads the current `upload_source_outputs`, and uses FFmpeg under
`exec.CommandContext` to create replacement publish parts. Output files are written below:

```text
upload-sources/<profile-id>/<upload-source-id>/edited/
```

The adapter must not build a shell command from user input. It uses structured arguments, validates all controlled
paths, and verifies generated media before returning metadata to the store. The current implementation uses stream
copy, so boundaries may align to nearby keyframes; frame-accurate re-encoding is intentionally not promised.

Bilibili resolves the current ready output rows when its job starts. Therefore a reviewed source whose manifest points
to `edited/...` uploads only edited parts. The pre-edit `parts/...` files are not fallback upload inputs.

COS likewise resolves the current output rows. After an edit, output-linked COS object state/key/compression metadata
is reset so compression and upload are regenerated from the edited file rather than reusing a pre-edit object.

---

## 5. Tencent COS

### 角色

Profile 可选的近期原文件滚动副本。

实现使用腾讯云官方 Go SDK：

```text
github.com/tencentyun/cos-go-sdk-v5
```

不启动额外 COS CLI/daemon。

Adapter 能力：

```text
PutObject
HeadObject
DeleteObject
GenerateSignedDownloadURL
```

COS Adapter 接收原始 output part。PutObject 返回成功后必须对同一个 object key
执行 HeadObject 验证，只有确认 COS 可见后才允许把对象标记为 `AVAILABLE`。COS object metadata 必须记录该对象
来自哪个 `upload_source_output`、源文件大小和上传对象大小，便于 UI 展示和排查问题。历史压缩 metadata 仍可显示。

Raw danmaku archive uses the same Tencent COS SDK Adapter, but bypasses FFmpeg compression and upload-source output
packaging. The source file is a closed `recording_files.kind = 'danmaku'` asset and is uploaded byte-for-byte to the
profile's COS prefix under `raw/`. This is intentionally an archival copy only; timeline alignment and downstream
metadata transformation remain out of scope until real raw samples are reviewed.

业务层自己维护：

- managed quota；
- object metadata；
- upload-source metadata；
- rolling policy；
- SOURCE_MISSING；
- retry。

### Prefix

每个 COS Storage Profile 使用受控 Prefix，例如：

```text
7grecorder/<profile-id>/
```

object key 由应用生成，用户不能提交任意删除 key。

新发布视频对象位于：

```text
<prefix>/videos/<China YYYY-MM-DD>/session-<daily ordinal>/p<output ordinal>.<source extension>
```

内部 profile/source ID 不再暴露为新 COS 视频目录。数据库中已有的 legacy key 仍由原记录管理。

---

## 6. ACRCloud File Scanning

Songs V1 uses ACRCloud File Scanning through its HTTPS API. It does not use the realtime SDK and does not expose the
access token to the browser.

Fixed contract for the first implementation:

```text
POST /api/fs-containers/{container_id}/files
GET  /api/fs-containers/{container_id}/files
GET  /api/fs-containers/{container_id}/files/{file_ids}
```

- API hosts are selected only from the configured regions `eu-west-1`, `us-west-2`, and `ap-southeast-1`.
- Upload is `multipart/form-data` with `file`, `data_type=audio`, and a deterministic `name`.
- The adapter searches that deterministic name before uploading, so a restart does not intentionally create a second
  provider file.
- The access token is stored as an encrypted SYSTEM credential with platform `acrcloud` and purpose
  `SONG_RECOGNITION`.
- Provider file ID and poll count are persisted as soon as submission succeeds. Poll states are normalized to
  processing, ready, no-result, or provider error.
- Analysis audio must remain below the provider's 500 MB upload limit. Songs V1 extracts mono 32 kHz MP3 at 64 kbps;
  multi-chunk analysis is deferred until the single-file production acceptance run is complete.
- Raw provider responses are retained only as recognition evidence. Credentials and request authorization data must
  never be written to evidence or logs.

The adapter parser is covered by sanitized fixtures in unit tests. The first real small-file response must be
sanitized and used to harden optional provider fields before large recordings are accepted.

## 7. 网易云/其他 Publisher

第一版不假设平台一定存在长期稳定公开上传 API。

所以统一通过：

```text
Publisher Port
```

隔离。

如果后续只能使用人工操作/半自动工具：

- 不影响 Recording/Songs；
- Job 可停留为 manual-required；
- 不为了某个平台改核心模型。

---

## 8. Adapter Error

所有 Adapter 映射到内部错误类别：

```text
TRANSIENT
AUTH
SOURCE_MISSING
PERMANENT
AMBIGUOUS
```

禁止 Application 依赖外部平台原始错误字符串做核心状态判断。

原始错误可以截断后写入 `last_error` 供排查。

---

## 9. Fixture Policy

外部工具接口变化是 AI Coding 最容易误判的地方。

仓库应保存脱敏 fixture：

```text
testdata/
├── bililiverecorder/
│   ├── webhook_session_started.json
│   ├── webhook_file_closed.json
│   └── ...
├── biliup/
│   ├── upload_success.txt
│   ├── upload_failure.txt
│   └── ...
└── cos/
    └── ...
```

规则：

- fixture 与仓库固定版本对应；
- 不含真实 Cookie/Secret/用户敏感数据；
- 升级外部组件时先更新 fixture/test，再改 Adapter；
- Coding Agent 不凭记忆猜外部 payload。
