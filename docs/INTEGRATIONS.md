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

### COS 压缩推荐方案

COS 上传前压缩使用 FFmpeg Adapter 的受控 preset，不允许前端提交任意 FFmpeg 参数。

默认目标：

```text
container: mp4
video:     libx264, yuv420p, CRF 23, preset medium
audio:     aac, 128k, keep existing audio stream count conservatively
flags:     +faststart
threads:   2 by default for COS-derived video encoding
```

示例语义，不作为 shell 拼接模板：

```text
ffmpeg -i input.flv -map 0:v:0 -map 0:a? -c:v libx264 -preset medium -crf 23 -threads 2 -pix_fmt yuv420p -c:a aac -b:a 128k -movflags +faststart output.mp4
```

选择理由：

- H.264/AAC/MP4 兼容性强，COS 下载和浏览器播放都稳定；
- CRF 23 通常能明显缩小直播录屏体积，同时不是激进破坏性压缩；
- `medium` 速度和压缩率平衡，后续可允许 SUPER_ADMIN 改为 `slow` 换取更小体积；
- `COS_COMPRESSION_THREADS` 默认 2，只限制 COS 派生压缩，为录制与管理 API 保留 CPU；
- 输出先写 Job temp，再用 ffprobe 验证可读、时长偏差在容忍范围内后才进入 COS 上传。

如果输入已经很小或压缩收益低，以后可以仍然上传原封装分片，或把压缩结果标记为 `SKIPPED_LOW_GAIN`。当前实现优先
采用稳定保守的“压缩成功才上传压缩件，压缩失败不损坏源文件”。

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

COS Adapter 可以接收原始 output part，也可以接收 FFmpeg Adapter 生成的压缩派生文件。压缩文件只存在于本地
受控 temp/derived path，上传成功后可以按 housekeeping 策略删除。PutObject 返回成功后必须对同一个 object key
执行 HeadObject 验证，只有确认 COS 可见后才允许把对象标记为 `AVAILABLE`。COS object metadata 必须记录该对象
来自哪个 `upload_source_output`、使用的压缩 preset、源文件大小和上传对象大小，便于 UI 显示压缩收益和排查问题。

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

---

## 6. 网易云/其他 Publisher

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

## 7. Adapter Error

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

## 8. Fixture Policy

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
