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

CLI 文本解析集中在 Adapter。

升级 biliup 必须更新 fixture。

---

## 4. FFmpeg / ffprobe

### 角色

```text
probe media
extract/cut audio
必要的轻量媒体处理
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
