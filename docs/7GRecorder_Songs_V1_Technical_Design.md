# 7GRecorder Songs V1 技术设计

> **Version:** V1.1  
> **Review focus:** Evidence-driven architecture, temporal-first aggregation, boundary estimation, Core+Guard analysis chunks, discontinuity handling, source retention lease.

# 1. 文档目标

本设计用于 7GRecorder 的自动歌曲识别与切片模块（Songs V1）。

目标体验：

> **一场 B 站直播结束 → 自动识别主播演唱的十几首歌曲 → 自动生成歌曲音频切片 → 管理后台逐首播放、确认、修改或拒绝。**

完整主链：

```text
直播结束
    ↓
Recording COMPLETED
    ↓
Songs 自动进入处理
    ↓
建立整场录播统一媒体时间轴
    ↓
生成低码率分析音频
    ↓
ACRCloud 自动识别歌曲
    ↓
Recognition Evidence 持久化
    ↓
Temporal Clustering（先按时间聚类）
    ↓
Metadata Resolution（再解析歌曲身份）
    ↓
Boundary Estimator（估计最终歌曲边界）
    ↓
生成 Song Draft
    ↓
从原始录像切高质量 M4A
    ↓
管理后台逐首播放
    ↓
Confirm / Reject / 修改边界 / Recut
```

---

# 2. V1 设计原则

## 2.1 核心架构主旨

Songs V1 不应把 ACRCloud 的识别结果直接视为最终事实。

核心原则：

> **AI / 第三方识别服务只是 Evidence Provider；7GRecorder 自己才负责时间轴、证据聚合、歌曲实体、人工真值、媒体切片与失败恢复。**

因此需要明确区分三层：

```text
Recognition Evidence
= Provider 实际返回了什么
= 尽量不可变，保留用于 Debug / Benchmark

Song / Performance
= 系统当前认为主播唱了什么
= 可以被聚合、人工修改、确认或拒绝

Clip Artifact
= 当前 Song 对应的媒体切片文件
= 可以根据边界变化反复重新生成
```

未来加入：

```text
YAMNet
Whisper
Humming Recognition
弹幕
歌词搜索
```

时，它们只作为新的 Evidence Provider 接入，不需要重写 Songs 主流程。


Songs V1 必须满足以下原则：

1. **Songs 与 Recording / Bilibili / COS 解耦**
   - 歌曲识别失败不能影响直播录制。
   - 歌曲切片失败不能影响 B 站上传或 COS 上传。
   - Songs 可以独立重试、重新处理。

2. **继续使用 SQLite Durable Job Queue**
   - 不引入 Redis。
   - 不引入 RabbitMQ / Kafka 等 MQ。
   - 继续沿用现有 Reconciler + Job Runtime 模式。

3. **重媒体任务必须受 Resource Guard 控制**
   - FFmpeg 任务保持低并发。
   - 2C / 4GB 服务器不允许同时跑多个重媒体任务。

4. **识别使用低码率分析音频**
   - 不上传整个原始视频。
   - 减少网络流量和第三方扫描成本。

5. **最终歌曲必须从原始录像切**
   - 低码率分析音频只负责识别。
   - 用户实际播放的歌曲使用原始录播音频重新编码。

6. **支持一场直播多个录像文件**
   - BililiveRecorder 可能产生多个 FLV。
   - Songs 只认识一条统一的 Recording Timeline。

7. **必须可以断点恢复**
   - 服务重启后能够继续等待 ACRCloud。
   - 外部扫描 ID、状态、结果必须持久化到 SQLite。
   - 不能只存在内存或 Job payload 中。

---

# 3. V1 技术范围

## 3.1 V1 使用的核心技术

```text
ACRCloud File Scanning
├── Audio Fingerprinting
├── Cover Song Identification
└── Music / Speech Detection

FFmpeg / ffprobe

SQLite Durable Job Queue

Go Backend

React 管理后台

Nginx X-Accel-Redirect
```

ACRCloud 使用：

```text
Traverse Scanning
+
Audio Fingerprinting
+
Cover Song Identification
```

三种能力的职责：

### Audio Fingerprinting

适合识别：

```text
直接播放原曲
原版伴奏
音乐播放器声音
与原录音高度相似的音乐
```

### Cover Song Identification

Songs V1 的主要能力之一。

适合识别：

```text
主播真人演唱
KTV 伴奏
Live 演唱
改 Key
不同歌手翻唱
部分编曲变化
```

这对于“中文主播直播唱歌”尤其重要。

### Music / Speech Detection

用于：

```text
辅助判断音乐区间
辅助判断歌曲结束后的聊天区间
后续辅助修正歌曲边界
```

V1 不单独根据 Music/Speech 自动创建 Unknown Song。

---

# 4. V1 暂不实现

为了避免第一版过重，以下内容全部留到 V2+：

```text
YAMNet
Whisper / whisper.cpp
Demucs
Humming Recognition
弹幕辅助识别
歌词搜索
自动人声分离
网易云自动发布
复杂 AI 多模型融合
```

V1 只解决：

```text
识别
→ 聚合
→ 自动切片
→ 网页试听
→ 人工 Review
```

---

# 5. 总体处理流程

```text
Recording COMPLETED
        │
        ▼
Songs Reconciler
        │
        ▼
PREPARE_SONG_ANALYSIS
        │
        ├── ffprobe 原始录播文件
        └── 建立 RecordingMediaTimeline
        │
        ▼
创建 Analysis Chunks
        │
        ▼
EXTRACT_SONG_ANALYSIS_CHUNK
        │
        └── FFmpeg 生成低码率 MP3
        │
        ▼
SUBMIT_ACRCLOUD_SCAN
        │
        ├── Audio Fingerprinting
        ├── Cover Song Identification
        └── Music / Speech Detection
        │
        ▼
FETCH_ACRCLOUD_RESULT
        │
        └── Poll ACRCloud 状态
        │
        ▼
FINALIZE_SONG_RECOGNITION
        │
        ├── 转换到 Recording 全局时间
        ├── 跨 Chunk 去重
        ├── Fingerprint + Cover 融合
        ├── 同曲结果聚合
        ├── 过滤误识别
        └── 创建 Song Draft
        │
        ▼
CUT_SONG_AUDIO
        │
        └── 从原始录像生成 M4A
        │
        ▼
Songs UI
        │
        ├── Play
        ├── Previous
        ├── Next
        ├── 修改 Start / End
        ├── Recut
        ├── Confirm
        └── Reject
```

---

# 6. RecordingMediaTimeline

## 6.1 为什么必须有这一层

一场直播可能产生：

```text
Recording 123

file_001.flv
file_002.flv
file_003.flv
```

歌曲可能：

```text
开始于 file_001.flv 最后一分钟
结束于 file_002.flv 前三分钟
```

如果 Songs 直接绑定单个文件，就会导致：

- 跨文件歌曲无法正常表示。
- ACRCloud 返回的时间无法稳定映射。
- 后续 Whisper / YAMNet / 弹幕都会重复做时间转换逻辑。

因此 Songs 内部必须统一使用：

```text
Recording Timeline

start = 0 ms
end   = recording total duration
```

---

## 6.2 时间轴示例

```text
0 ms
│
├── file_001.flv
│   0
│   →
│   7,201,432 ms
│
├── file_002.flv
│   7,201,432
│   →
│   14,402,781 ms
│
└── file_003.flv
    14,402,781
    →
    18,920,550 ms
```

Song 永远只保存：

```text
start_ms
end_ms
```

例如：

```text
Song:
start_ms = 7,188,000
end_ms   = 7,411,000
```

Songs 不需要知道歌曲跨了几个源文件。

---

## 6.3 获取真实 Duration

Songs 开始前：

```text
ffprobe each CLOSED recording file
```

获取真实：

```text
duration_ms
```

优先使用 ffprobe 结果，而不是完全信任录制期间估算值。

生成 source map：

```json
{
  "files": [
    {
      "recording_file_id": 101,
      "timeline_start_ms": 0,
      "timeline_end_ms": 7201432,
      "duration_ms": 7201432
    },
    {
      "recording_file_id": 102,
      "timeline_start_ms": 7201432,
      "timeline_end_ms": 14402781,
      "duration_ms": 7201349
    }
  ]
}
```

---

## 6.4 Timeline Range Resolver

建议实现一个独立组件：

```text
RecordingMediaTimeline.ResolveRange(start_ms, end_ms)
```

例如：

```text
ResolveRange(
    7,188,000,
    7,411,000
)
```

返回：

```json
[
  {
    "recording_file_id": 101,
    "source_start_ms": 7188000,
    "source_end_ms": 7201432
  },
  {
    "recording_file_id": 102,
    "source_start_ms": 0,
    "source_end_ms": 209568
  }
]
```

FFmpeg 切片器只负责处理这些 source ranges。

---

## 6.5 Timeline Discontinuity

Recording Timeline 不能只记录“媒体连续时间”，还必须知道真实直播时间是否发生过明显断流。

例如：

```text
file_001
22:00 → 23:00

断流 3 分钟

file_002
23:03 → 00:30
```

媒体时间轴可以继续压缩为连续播放，但 Timeline Entry 应额外记录：

```text
wallclock_start
wallclock_end

timeline_start_ms
timeline_end_ms

discontinuity_before
wallclock_gap_ms
```

建议规则：

```text
如果 wallclock gap 超过允许阈值
→ discontinuity_before = true
```

Aggregator 默认不得跨明显 discontinuity 自动合并歌曲。

这是为了避免断流前后恰好出现同一首歌时被错误合并成一次连续演唱。

---

# 7. Analysis Audio

## 7.1 禁止直接上传完整视频

不要：

```text
5 GB FLV
→ ACRCloud
```

应该：

```text
原始录像
    ↓
FFmpeg
    ↓
低码率分析 MP3
    ↓
ACRCloud
```

---

## 7.2 推荐参数

V1 默认：

```text
Codec        MP3
Channels     Mono
Sample Rate  32 kHz
Bitrate      64 kbps
Core Duration 2 hours
Guard         30 seconds（每个 Core 前后各扩展 30 秒）
```

推荐把 Analysis Chunk 定义为 **Core + Guard**，不要只写成“30 秒 overlap”。

例如：

```text
Core #1:
00:00:00 → 02:00:00

Analysis #1:
00:00:00 → 02:00:30

Core #2:
02:00:00 → 04:00:00

Analysis #2:
01:59:30 → 04:00:30

Core #3:
04:00:00 → 06:00:00

Analysis #3:
03:59:30 → 06:00:00
```

定义：

```text
core_duration = 2 hours
guard_before  = 30 sec
guard_after   = 30 sec
```

因此相邻 Analysis 实际可能有最多 60 秒重叠。

Guard 用于避免一首歌刚好跨 Core 边界而导致识别被截断。

后续 Evidence 可以使用：

```text
match midpoint belongs to which Core
```

来确定主要归属 Chunk；相邻 Chunk 的重复结果仍可作为辅助证据。

---

## 7.3 文件大小估算

64 kbps：

```text
约 28.8 MB / hour
```

两小时：

```text
约 57.6 MB
```

六小时直播：

```text
约 170 MB
```

相比几个 GB 的录像文件非常小。

---

## 7.4 临时目录

```text
/data/7grecorder/temp/
└── songs/
    └── recording-123/
        ├── analysis-0001.mp3
        ├── analysis-0002.mp3
        └── analysis-0003.mp3
```

处理完成后删除。

上传策略建议：

```text
Analysis Chunk < 100 MB
→ Direct Upload

Analysis Chunk >= 100 MB
→ Presigned Upload
```

默认 2 小时 / 64 kbps 的 Analysis Chunk 约 57.6 MB，因此正常情况下 Direct Upload 足够，V1 不必强制所有上传都走更复杂的 Presigned 流程。

---

# 8. ACRCloud 配置

## 8.1 Container

创建长期复用的：

```text
File Scanning Container
```

不要每场直播创建新 Container。

建议：

```text
Audio Engine:
Audio Fingerprinting
+
Cover Song Identification

Scanning:
Traverse Scanning

Music/Speech Detection:
Enabled
```

如果通过 API 表示：

```json
{
  "audio_type": "linein",
  "engine": 3,
  "policy": {
    "type": "traverse",
    "interval": 0,
    "rec_length": 10
  },
  "music_detection": 1
}
```

实际字段以接入时 ACRCloud 当前 API 文档为准。

---

# 9. V1 使用 Poll，而不是 Callback

ACRCloud 支持 Callback，但 V1 建议不用。

原因：

```text
少一个公网 Callback Endpoint
少一套签名 / 安全处理
更符合当前轻量架构
重启恢复更直观
```

流程：

```text
SUBMIT_ACRCLOUD_SCAN
        ↓
保存 provider_file_id
        ↓
Job 结束
        ↓
Songs Reconciler 定期检查
        ↓
FETCH_ACRCLOUD_RESULT
```

Provider 状态示意：

```text
PROCESSING
READY
NO_RESULT
ERROR
```

Songs Reconciler 根据持久化状态决定是否继续 enqueue。

建议持久化：

```text
last_polled_at
next_poll_at
poll_attempts
```

并使用简单 backoff：

```text
30 sec
→ 1 min
→ 2 min
→ 5 min
→ 5 min ...
```

避免 Reconciler 每次运行都重复创建 Fetch Job。

---

# 10. 为什么 Provider ID 不能只放 Job Payload

错误设计：

```text
jobs.payload_json
{
    "provider_file_id": "..."
}
```

然后 Job 完成以后就依赖历史 Job 来恢复流程。

正确设计：

```text
song_analysis_chunks.provider_file_id
```

外部扫描任务属于：

```text
业务状态
```

不是：

```text
Job Runtime 临时参数
```

服务重启后：

```text
SQLite
↓
找到 PROCESSING chunk
↓
provider_file_id 仍存在
↓
继续查询
```

---

# 11. 建议新增数据表

## 11.1 song_analysis_runs

```text
song_analysis_runs
------------------

id

recording_id

revision

provider
    acrcloud

algorithm_version
provider_config_snapshot_json

status
    PENDING
    PROCESSING
    PARTIAL
    REVIEW_REQUIRED
    COMPLETED
    FAILED
    SOURCE_MISSING

source_map_json

started_at
completed_at

last_error

created_at
updated_at
```

`source_map_json` 保存本次分析使用的 RecordingMediaTimeline snapshot。

---

## 11.2 song_analysis_chunks

```text
song_analysis_chunks
--------------------

id

run_id

chunk_index

timeline_start_ms
timeline_end_ms

analysis_relative_path

analysis_status
    PENDING
    PROCESSING
    AVAILABLE
    FAILED
    DELETED

provider
    acrcloud

provider_file_id

provider_state
    NONE
    SUBMITTED
    PROCESSING
    READY
    NO_RESULT
    FAILED

provider_result_json

last_polled_at
next_poll_at
poll_attempts

status

last_error

created_at
updated_at
```

---

# 12. Songs 表建议

现有 Songs 继续作为最终歌曲实体。

建议语义：

```text
songs
-----

id

recording_id

title
artist

detected_start_ms
detected_end_ms

start_ms
end_ms

confidence

status
    DRAFT
    CONFIRMED
    REJECTED

local_audio_status
    NONE
    PENDING
    PROCESSING
    AVAILABLE
    FAILED
    SOURCE_MISSING
    DELETED

audio_relative_path

clip_revision

created_at
updated_at
```

---

# 13. detected_start/end 与 start/end 的区别

ACRCloud 原始识别：

```text
detected_start_ms
detected_end_ms
```

例如：

```text
02:10:29
→
02:14:07
```

系统自动增加 padding：

```text
start_ms
end_ms
```

例如：

```text
02:10:21
→
02:14:17
```

以后用户手动修改：

```text
02:10:17
→
02:14:21
```

系统仍然保留：

```text
AI 原始检测边界
```

便于：

- Debug。
- 后续算法评估。
- 比较人工调整幅度。
- V2 模型优化。

---

# 14. Recognition Evidence 与 song_candidates

V1 建议新增正式的 Evidence 表，而不是只把所有中间信息塞进 `song_candidates`。

推荐新增：

```text
song_recognition_matches
```

字段：

```text
id

run_id
chunk_id

engine
    FINGERPRINT
    COVER

provider_track_id

acrid
isrc

title
artist

raw_start_ms
raw_end_ms

global_start_ms
global_end_ms

provider_score

raw_evidence_json

created_at
```

该表表达：

> Provider 实际返回了什么。

原则上这些 Evidence 一旦写入就尽量不可变。

这样以后可以完整追溯为什么识别错误、漏歌或不同引擎产生冲突。

## 14.1 song_candidates 的职责

`song_candidates` 保留，但职责调整为：

> 对某个已经形成的 Song / Performance，保存多个“歌曲身份候选”。

例如：

```text
Song #531

Candidate 1
source = ACRCLOUD_COVER
title  = 晴天
artist = 周杰伦

Candidate 2
source = ACRCLOUD_FINGERPRINT
title  = 晴天（KTV Version）
artist = 某伴奏发行方
```

未来可以继续加入：

```text
WHISPER_LYRICS
HUMMING
DANMAKU
MANUAL
```

因此：

```text
song_recognition_matches
= 原始识别证据

song_candidates
= 聚合后的歌曲身份候选
```

两者职责不同。

---

# 15. ACRCloud Result Normalizer

所有 ACRCloud 返回结果先转换成内部统一结构：

```text
RawSongMatch
```

建议：

```go
type RawSongMatch struct {
    Engine       string

    Title        string
    Artist       string

    ACRID        string
    ISRC         string

    StartMS      int64
    EndMS        int64

    ProviderScore *float64

    EvidenceJSON []byte
}
```

其中 `StartMS / EndMS` 必须已经转换为：

```text
Recording 全局时间
```

后续 Aggregator 不再关心 Analysis Chunk。

---

# 16. 时间归一化

例如：

```text
Analysis Chunk global start:
01:59:30
```

ACRCloud 返回：

```text
offset:
662.5 sec

played_duration:
213.2 sec
```

那么：

```text
global start

=
01:59:30
+
00:11:02.5

=
02:10:32.5
```

End：

```text
02:10:32.5
+
00:03:33.2

=
02:14:05.7
```

进入 RawSongMatch 后保存毫秒：

```text
StartMS
EndMS
```

---

# 17. Temporal-first Aggregation

对于主播翻唱场景，不建议把 ACRID 或 ISRC 作为最优先的聚合依据。

Fingerprint 可能命中 KTV 伴奏版、翻唱录音版或不同发行版本，而 Cover Song Identification 可能返回原作品。此时 ACRID、ISRC，甚至 artist 都可能不同，但实际上对应同一次主播演唱。

因此 V1 推荐采用：

```text
Raw Recognition Matches
        ↓
Temporal Clustering
        ↓
Metadata Resolution
        ↓
Performance Segment
```

## 17.1 Temporal Clustering

先按照 Recording 全局时间建立 Cluster。

例如：

```text
Evidence A
02:10:29 - 02:14:07
晴天

Evidence B
02:10:34 - 02:13:58
晴天（KTV Version）

Evidence C
02:10:31 - 02:14:03
晴天
```

因为三个 Evidence 时间高度重叠，应首先形成同一个 Performance Cluster。

推荐初始规则：

```text
time ranges overlap substantially
OR
gap <= 15 seconds
```

同时：

```text
不得跨明显 discontinuity 自动 merge
```

## 17.2 Metadata Resolution

Cluster 建立之后，再判断歌曲身份。

Metadata Resolution 可以综合：

```text
title compatibility
artist compatibility
ACRID
ISRC
Fingerprint score
Cover support
持续时间
```

ACRID / ISRC 不再作为硬编码的唯一优先身份键，而是多个证据字段之一。

V1 仍建议避免过度 fuzzy matching：

```text
宁可生成两个待 Review 的 Song
也不要把两次不同演唱错误合并
```

---

# 18. Boundary Estimator

ACRCloud 返回的 `offset / played_duration` 应视为 **识别证据窗口**，而不是天然等于主播完整演唱的真实开始和结束。

因此在 Aggregator 后增加独立组件：

```text
BoundaryEstimator
```

流程：

```text
Performance Cluster
        ↓
identified interval
        ↓
Boundary Estimator
        ↓
detected_start_ms / detected_end_ms
        ↓
start_ms / end_ms
```

## 18.1 V1 边界规则

V1 不需要额外 ML 模型。

推荐：

```text
identified_start
↓
向前最多搜索 15~20 秒
↓
参考 Music/Speech 最近合理音乐边界

identified_end
↓
向后最多搜索 15~20 秒
↓
参考 Music/Speech 最近合理聊天/非音乐边界
```

找不到可靠边界时使用 fallback：

```text
start = identified_start - 8 sec
end   = identified_end + 10 sec
```

必须限制 maximum expansion，防止持续 BGM 造成歌曲被错误扩展成很长的区间。

V2 可以替换实现：

```text
BoundaryEstimatorV1
Music/Speech Rules

BoundaryEstimatorV2
YAMNet Singing

BoundaryEstimatorV3
Multi-model
```

上层 Songs 流程不需要改变。

## 18.2 detected 与 clip 边界

建议：

```text
detected_start_ms
detected_end_ms
```

保存 Boundary Estimator 得出的系统检测区间。

而：

```text
start_ms
end_ms
```

保存当前真正用于播放 / 切片的边界。用户手动修改 start/end 不覆盖系统历史检测值。

---

# 19. 跨 Chunk 去重

因为 Analysis Chunk 有：

```text
30 sec guard
```

同一首歌可能被两个 Chunk 同时识别。

处理顺序：

```text
Provider Result
        ↓
转换到 Recording 全局时间
        ↓
Identity Match
        ↓
时间区间 Match
        ↓
Merge
```

因为 Analysis 使用 Core + Guard，同一 Evidence 可能同时出现在相邻 Chunk。

推荐：

```text
先转换到 Recording 全局时间
↓
按时间聚类
↓
使用 Core ownership 选择主 Evidence
↓
相邻 Chunk 的结果作为辅助证据
```

因此不需要为每一对 Chunk 编写特殊-case 合并代码。

---

# 20. Minimum Match Duration

为了避免：

```text
直播中播放几秒提示音乐
BGM 短暂命中
主播哼两句
系统音效误匹配
```

V1 建议：

```text
minimum recognized span
=
20 sec
```

小于 20 秒：

```text
默认不自动创建 Song
```

如果多个连续结果合并以后：

```text
>= 20 sec
```

则可以正常创建。

该参数需要配置化，后续用真实直播 benchmark 调整。

---

# 21. Music / Speech Detection 的职责

V1：

```text
Music/Speech
≠
Song Detector
```

不要：

```text
Music detected
→ 自动创建 Unknown Song
```

否则：

```text
直播 BGM
游戏音乐
片头
提示音乐
休息音乐
```

都会变成大量 Unknown Song。

V1 只为：

```text
Fingerprint
或
Cover Song
```

真正识别出的内容创建 Song。

Music/Speech 仅保存为 evidence，未来可用于：

```text
边界辅助
V2 Unknown Song Detection
YAMNet 融合
```

---

# 22. Confidence 设计

`songs.confidence` 不应该简单等于某一个 ACRCloud Score。

因为：

```text
Fingerprint
```

和：

```text
Cover Song
```

返回的评分语义可能不同。

因此：

```text
confidence
=
7GRecorder 自己的综合置信度
```

推荐 V1 内部可以使用：

```text
HIGH
MEDIUM
LOW
```

数据库如果仍然是 numeric，可以建立内部映射，但 UI 推荐显示：

```text
HIGH
Fingerprint + Cover
```

而不是：

```text
97.384%
```

避免制造虚假精度。

---

## 22.1 V1 Confidence Rule

示例：

```text
Fingerprint + Cover 同时命中
→ HIGH

Cover 单独命中
且 duration >= 60 sec
→ HIGH

Fingerprint score >= 90
且 duration >= 30 sec
→ HIGH

Cover 单独命中
20 ~ 60 sec
→ MEDIUM

Fingerprint 中等分
→ MEDIUM

只有短暂弱匹配
→ LOW / Filter
```

实际阈值在真实数据测试后调整。

---

# 23. Song 创建

Recognition Finalize 后：

```text
INSERT songs
```

例如：

```text
id:
531

recording_id:
123

title:
晴天

artist:
周杰伦

detected_start_ms:
7829000

detected_end_ms:
8047000

start_ms:
7821000

end_ms:
8057000

status:
DRAFT

local_audio_status:
PENDING

clip_revision:
1
```

此时即使音频还没切完，网页可以先显示：

```text
晴天
周杰伦

02:10:21
-
02:14:17

HIGH
Fingerprint + Cover

正在生成音频...
```

---

# 24. 自动切片策略

不是所有识别结果都必须立即消耗 FFmpeg 资源。

推荐：

```text
HIGH
→ 自动 CUT

MEDIUM
→ 自动 CUT

LOW
→ 生成 DRAFT，但默认不自动 CUT

FILTERED
→ 不进入普通 Songs 列表
```

这样可以避免一次直播出现大量弱匹配后，服务器继续生成几十个无意义切片。

是否自动切片由：

```text
minimum_match_sec
+
confidence level
+
auto_cut policy
```

共同决定。

---

# 25. 最终歌曲切片

低码率 Analysis MP3：

```text
只能用于识别
```

最终歌曲必须：

```text
原始 FLV
↓
RecordingMediaTimeline
↓
FFmpeg
↓
M4A
```

推荐：

```text
AAC
Stereo
保留合理原始采样率
192 kbps
```

---

# 26. 单文件切片

例如 Song：

```text
start_ms:
02:10:21

end_ms:
02:14:17
```

完全位于：

```text
file_002.flv
```

则直接：

```text
file_002.flv
↓
FFmpeg
↓
531.m4a
```

---

# 27. 跨文件切片

例如：

```text
Song start:
file_001 最后 50 秒

Song end:
file_002 前 3 分钟
```

Timeline Resolver 返回两个 source ranges。

流程：

```text
file_001 range
        ↓
extract temporary audio part A

file_002 range
        ↓
extract temporary audio part B

A + B
↓
concat
↓
encode final M4A
```

最终：

```text
531.m4a
```

对于用户来说仍然是一首连续歌曲。

---

# 28. Song 文件路径

不要使用：

```text
晴天-周杰伦.m4a
```

作为实际文件路径。

推荐：

```text
/data/7grecorder/songs/
└── <profile-id>/
    └── <recording-id>/
        ├── 531.m4a
        ├── 532.m4a
        └── 533.m4a
```

原因：

- 中文文件名没有必要。
- 避免特殊符号。
- 避免同名歌曲冲突。
- 修改 title / artist 不需要 rename。
- 更适合程序管理。

数据库只保存：

```text
audio_relative_path
```

---

# 29. Recut

用户修改：

```text
start_ms
end_ms
```

以后：

```text
clip_revision++
```

例如：

```text
1 → 2
```

Job key：

```text
song:531:cut:r2
```

而不是始终：

```text
song:531:cut
```

这样每次切片修订都有明确幂等键。

---

# 30. Job 设计

推荐 Job：

| Job | Resource | 作用 |
|---|---|---|
| PREPARE_SONG_ANALYSIS | MEDIA | ffprobe + Timeline + 创建 Analysis Run |
| EXTRACT_SONG_ANALYSIS_CHUNK | MEDIA | 生成低码率 MP3 |
| SUBMIT_ACRCLOUD_SCAN | NETWORK | 上传并提交 ACRCloud |
| FETCH_ACRCLOUD_RESULT | NETWORK | 查询并保存结果 |
| FINALIZE_SONG_RECOGNITION | LIGHT | Normalize + Merge + 建 Song |
| CUT_SONG_AUDIO | MEDIA | 从原始录像切 M4A |
| CLEANUP_SONG_ANALYSIS | MAINTENANCE | 删除临时分析文件和远端扫描文件 |

---

# 31. Resource Guard

当前服务器为轻量环境时，建议：

```text
MEDIA concurrency:
1
```

即：

```text
同一时间最多一个：

FFmpeg Analysis Extraction
或
Song Cutting
```

Network Job 可以独立。

例如：

```text
MEDIA    = 1
NETWORK  = 1
AI       = 1
```

不要同时切十几首歌。

十几首歌按 Job Queue 顺序逐个切即可。

---

# 32. 不做 Job DAG

不增加：

```text
job_dependencies
workflow_nodes
```

继续使用 Reconciler。

例如：

```text
Songs Reconciler
```

发现：

```text
Analysis Chunk
analysis_status = PENDING
```

则：

```text
enqueue EXTRACT
```

发现：

```text
analysis_status = AVAILABLE
provider_state = NONE
```

则：

```text
enqueue SUBMIT
```

发现：

```text
provider_state = PROCESSING
```

则在合适时间：

```text
enqueue FETCH
```

发现：

```text
所有 chunks final
且 run 未 finalized
```

则：

```text
enqueue FINALIZE
```

发现：

```text
Song
local_audio_status = PENDING
```

则：

```text
enqueue CUT
```

---

# 33. Recording Song Processing 状态

推荐继续使用：

```text
DISABLED
PENDING
PROCESSING
REVIEW_REQUIRED
COMPLETED
SKIPPED
FAILED
SOURCE_MISSING
```

状态机：

```text
Songs disabled
↓
DISABLED
```

Recording 完成：

```text
PENDING
```

开始第一个 Songs Job：

```text
PROCESSING
```

识别完成且找到歌曲：

```text
REVIEW_REQUIRED
```

所有歌曲：

```text
CONFIRMED
或
REJECTED
```

以后：

```text
COMPLETED
```

识别完成但 0 首：

```text
COMPLETED
```

源文件已经不存在：

```text
SOURCE_MISSING
```

全部扫描失败：

```text
FAILED
```

---

# 34. Partial Success

例如：

```text
3 Analysis Chunks
```

其中：

```text
Chunk 1 READY
Chunk 2 READY
Chunk 3 FAILED
```

不要整场直接：

```text
FAILED
```

而是：

```text
song_analysis_run.status
=
PARTIAL
```

已经识别出的歌曲仍然正常生成。

Recording：

```text
REVIEW_REQUIRED
```

UI 提示：

```text
Songs recognition partially completed.

2 / 3 analysis chunks succeeded.
```

用户可以：

```text
Retry Failed Chunk
```

---

# 35. Songs UI

## 34.1 Recording Songs 页面

示例：

```text
Recording #123
2026-09-12 七宫直播

12 Songs

Review Required
```

列表：

```text
▶ 01  晴天
       周杰伦

       02:10:21 - 02:14:17
       HIGH
       Fingerprint + Cover


  02  泡沫
       G.E.M.

       02:38:11 - 02:42:03
       HIGH
       Cover


  03  小幸运
       田馥甄

       03:01:22 - 03:05:45
       MEDIUM
       Cover
```

---

## 34.2 单播放器模式

不要渲染：

```text
12 个独立 <audio>
```

推荐：

```text
一个 Single Song Player
+
一个 Song List
```

播放器：

```text
← Previous

晴天
周杰伦

00:43
━━━━━━━━━━━━━━━━━━
03:56

Play / Pause

Next →
```

点击列表：

```text
player source 切换
```

播放结束：

```text
自动 Next
```

---

# 36. Song Review

每首歌至少支持：

```text
Play

Title
Artist

Start
End

Recut

Confirm

Reject
```

例如：

```text
Title:
晴天

Artist:
周杰伦

Start:
02:10:21

End:
02:14:17

[Recut]

[Confirm]
[Reject]
```

---

# 37. 修改行为

修改：

```text
title
artist
```

只更新 metadata：

```text
不需要重新切片
```

修改：

```text
start_ms
end_ms
```

则：

```text
clip_revision++

local_audio_status = PENDING

enqueue CUT_SONG_AUDIO
```

---

# 38. Confirm / Reject

识别完成默认：

```text
DRAFT
```

用户确认：

```text
CONFIRMED
```

用户判断识别错误或不是歌曲：

```text
REJECTED
```

后续网易云发布等模块只消费：

```text
CONFIRMED
```

---

# 39. Songs 总曲库

除 Recording 详情页之外，建议 V1 后半段增加：

```text
/songs
```

展示所有历史 Songs：

```text
2026-09-12
晴天
泡沫
小幸运

2026-09-11
如果可以
唯一
...
```

支持：

```text
按主播/Profile 筛选

按日期筛选

按状态筛选

搜索 title / artist

逐首播放
```

---

# 40. Audio API

推荐：

```text
GET
/api/songs/:songId/audio
```

Go Backend：

```text
Authentication
↓
Authorization
↓
Verify Song ownership
↓
Verify path inside songs root
↓
X-Accel-Redirect
```

Nginx 实际发送：

```text
M4A
```

这样支持：

```text
HTTP Range
```

拖动播放器进度条不需要重新下载整首歌。

---

# 41. V1 API

建议：

```text
GET
/api/recordings/:recordingId/songs

GET
/api/songs

GET
/api/songs/:songId

GET
/api/songs/:songId/audio

PATCH
/api/songs/:songId

POST
/api/songs/:songId/confirm

POST
/api/songs/:songId/reject

POST
/api/songs/:songId/recut

POST
/api/recordings/:recordingId/songs/reprocess
```

可选：

```text
POST
/api/song-analysis/:runId/retry-failed
```

---

# 42. PATCH Song

示例：

```json
{
  "title": "晴天",
  "artist": "周杰伦",
  "start_ms": 7821000,
  "end_ms": 8057000
}
```

逻辑：

```text
only title / artist changed
→ metadata update

start / end changed
→ clip_revision++
→ audio PENDING
→ enqueue recut
```

---

# 43. ACRCloud Credential

继续复用统一 credentials 系统。

例如：

```text
platform:
acrcloud

purpose:
AI_RECOGNITION

scope:
SYSTEM
```

保存：

```text
Access Token
```

Secret 必须：

```text
encrypted at rest
```

API：

```text
绝对不能返回明文 Token
```

普通 Manager 只看到：

```text
Provider:
ACRCloud

Status:
Configured
```

---

# 44. Songs Processing Profile

建议建立：

```text
song_processing_profiles
```

字段：

```text
id

recording_profile_id

credential_id

enabled

auto_process

provider
    acrcloud

container_region

container_id

analysis_chunk_sec
    default 7200

analysis_guard_sec
    default 30

minimum_match_sec
    default 20

pre_roll_ms
    default 8000

post_roll_ms
    default 10000

auto_cut
    default true

created_at
updated_at
```

后续可以增加：

```text
enable_humming
enable_whisper
enable_yamnet
```

但 V1 不需要。

---

# 45. Cleanup

Analysis 结果成功保存到 SQLite 以后：

```text
analysis MP3
```

就不需要长期存在。

处理结束：

```text
/data/7grecorder/temp/songs/recording-123
```

整体清理。

如果 ACRCloud 支持删除远端扫描文件：

```text
CLEANUP_SONG_ANALYSIS
```

顺带删除。

---

# 46. 本地原始录像删除后 Songs 怎么办

Song M4A：

```text
不跟随 Recording 本地录像一起滚动删除
```

例如：

```text
Recording:
8 GB

上传 Bilibili 完成
COS 完成
本地存储不足
→ 删除
```

但是：

```text
531.m4a  5 MB
532.m4a  6 MB
533.m4a  5 MB
...
```

继续保存。

这样半年后：

```text
原视频已经不在服务器
```

用户仍然可以进入：

```text
Songs
```

听当时切出的歌曲。

---

# 47. Source Missing

如果：

```text
歌曲还没有切片
```

但原始 Recording 已经被删除：

```text
local_audio_status
=
SOURCE_MISSING
```

已经存在的 Song metadata 继续保留。

UI 显示：

```text
Audio unavailable:
source recording has been deleted.
```

不要删除 Song 数据。

---

# 48. Source Retention Lease

Songs 与 Recording Cleanup 是独立自治模块，因此必须避免竞态：

```text
Recognition 完成
↓
创建 12 个 Song
↓
准备 CUT

同时：
Recording Cleanup
↓
发现磁盘不足
↓
删除原始录像
```

这会导致所有待切歌曲进入 SOURCE_MISSING。

因此建议引入明确的：

```text
Source Retention Lease
```

语义：

```text
Songs 开始需要原始录播
→ Acquire Lease

所有需要自动生成的 Clip
已经 AVAILABLE / FAILED final
→ Release Lease
```

注意：不需要等用户 Confirm / Reject。人工 Review 可以之后慢慢进行。

Lease 只保护自动识别 + 自动切片真正需要源文件的生命周期。

实现方式可以是 Recording 上的计数/状态，也可以是独立 lease 表，但必须保证：

```text
Recording Cleanup 在删除源录像前
必须检查是否存在 active source lease
```

---

# 49. 自动清理与 Rolling Storage 的关系

Recording Cleanup 在删除源录像前，不应该要求：

```text
所有 Song 必须 Confirm
```

否则用户不 Review 就会阻止录像滚动清理。

建议只要：

```text
Song recognition 已经完成
且自动切片任务已经结束
```

就可以视为 Songs 不再需要源录像。

如果 Songs 模块：

```text
Disabled
```

则完全不参与录播清理条件。

---

# 50. 幂等性

所有 Job 必须有稳定 idempotency key。

示例：

```text
song-analysis:<recording-id>:prepare:r1

song-analysis:<run-id>:chunk:<index>:extract

song-analysis:<chunk-id>:submit

song-analysis:<chunk-id>:fetch:<provider-revision>

song-analysis:<run-id>:finalize

song:<song-id>:cut:r<clip-revision>
```

重复执行必须安全。

---

# 51. Reprocess

用户可以对 Recording：

```text
Reprocess Songs
```

不要覆盖旧 run。

应该：

```text
revision++
```

生成：

```text
song_analysis_runs

revision = 2
```

旧 run 保留用于：

- Debug。
- 算法对比。
- 未来 benchmark。

是否自动替换旧 Song，需要后续定义。

V1 推荐：

```text
Reprocess 前警告：
会重新生成 DRAFT Songs。

CONFIRMED Songs 不自动覆盖。
```

---

# 52. 错误处理

## ffprobe failed

```text
Recording source invalid
→ chunk not created
→ run FAILED / PARTIAL
```

## FFmpeg analysis extraction failed

```text
chunk FAILED
→ retry
```

## ACRCloud upload failed

```text
provider_state stays NONE/SUBMISSION_FAILED
→ network retry
```

## ACRCloud processing timeout

```text
保持 provider_file_id
→ later retry query
```

不要重新上传相同文件，除非确认远端任务不存在。

## ACRCloud no result

```text
provider_state = NO_RESULT
```

这是正常 final 状态，不是系统错误。

## Song cut failed

```text
Song metadata 保留

local_audio_status = FAILED

允许 Retry
```

---

# 53. Logging

所有日志至少包含：

```text
recording_id

song_analysis_run_id

chunk_id

song_id

job_id

provider_file_id
```

示例：

```text
songs.analysis.submit

recording_id=123
run_id=45
chunk_id=91
provider=acrcloud
provider_file_id=xxx
```

方便排查。

---

# 54. Metrics

V1 可以先记录简单 metrics：

```text
songs_recognition_duration_seconds

songs_detected_total

songs_confirmed_total

songs_rejected_total

songs_recut_total

acrcloud_chunk_success_total

acrcloud_chunk_failed_total

song_cut_duration_seconds
```

未来可以计算：

```text
AI Recognition Precision
```

例如：

```text
Confirmed
/
(Confirmed + Rejected)
```

---

# 55. Benchmark

上线前建议取真实主播录播进行 benchmark。

测试集至少：

```text
20 ~ 50 首实际演唱
```

分类：

```text
标准 KTV 伴奏

升 Key / 降 Key

清唱

吉他 / 钢琴伴奏

边唱边说话

唱得不太准

直接播放原曲
```

记录：

```text
Ground Truth

Fingerprint Result

Cover Result

Final Aggregated Result

Start Error

End Error
```

重点观察：

```text
中文歌曲识别率

误识别率

歌曲漏识别率

边界误差

ACRCloud Cover 对主播真实演唱的效果
```

---

# 56. V1 开发阶段

## S1 — RecordingMediaTimeline

完成：

```text
ffprobe
Timeline Builder
Range Resolver
```

验收：

```text
任意全局时间段
都可以正确映射回一个或多个 FLV
```

这是整个 Songs 模块最重要的基础。

---

## S2 — Cross-file Audio Cutter

完成：

```text
单 FLV 切片

跨 FLV 切片

M4A 输出
```

验收：

```text
给定：
start_ms
end_ms

无论跨多少 RecordingFile
都能生成连续正确音频
```

---

## S3 — Songs Database

新增：

```text
song_processing_profiles

song_analysis_runs

song_analysis_chunks
```

完善：

```text
songs

song_candidates
```

完成 migrations。

---

## S4 — Analysis Audio

实现：

```text
2 hour chunks

30 sec guard

MP3 64 kbps mono
```

验收：

```text
长直播可以自动生成所有 analysis chunks
```

---

## S5 — ACRCloud Adapter

独立 Provider Interface：

```go
type SongRecognitionProvider interface {
    Submit(...)
    GetStatus(...)
    GetResult(...)
    Delete(...)
}
```

实现：

```text
ACRCloudProvider
```

不要把 ACRCloud API 逻辑散落到 Songs Service。

---

## S6 — Result Normalizer

实现：

```text
Fingerprint Parser

Cover Parser

Music/Speech Parser

Global Timeline Converter
```

输出：

```text
RawSongMatch[]
```

---

## S7 — Aggregator

实现：

```text
Identity

Merge

Overlap Dedup

Minimum Duration

Padding

Confidence
```

验收：

```text
同一首歌曲多个 provider/chunk 结果
只生成一首 Song
```

---

## S8 — Auto Cut

生成 Song 后：

```text
local_audio_status = PENDING
```

Reconciler：

```text
enqueue CUT_SONG_AUDIO
```

完成：

```text
AVAILABLE
```

---

## S9 — Recording Songs UI

实现：

```text
Song List

Single Player

Previous / Next

Play / Pause

Title / Artist Edit

Start / End Edit

Recut

Confirm

Reject
```

---

## S10 — Global Songs Library

实现：

```text
/songs
```

支持：

```text
历史歌曲

搜索

筛选

连续播放
```

---

## S11 — Cleanup / Recovery

测试：

```text
服务处理中重启

FFmpeg 失败

ACRCloud 超时

ACRCloud 无结果

部分 Chunk 失败

源文件被删除

Recut

Reprocess
```

---

# 57. 推荐开发顺序

严格建议：

```text
S1
↓
S2
↓
S3
↓
S4
↓
S5
↓
S6
↓
S7
↓
S8
↓
S9
↓
S10
↓
S11
```

不要直接让 Coding Agent 一次性写完整 Songs V1。

尤其：

```text
S1 RecordingMediaTimeline
+
S2 Cross-file Audio Cutter
```

必须先单独测试透。

否则后面：

```text
ACRCloud 识别再准
```

只要：

```text
全局时间 ↔ FLV 时间
```

有偏移，最终切出来的歌都会错。

---

# 58. V1 最终用户体验

配置一次：

```text
Songs Module

Enabled:
Yes

Auto Process:
Yes

Provider:
ACRCloud

Status:
Configured
```

以后：

```text
21:00
主播开播

↓

7GRecorder 自动录制

↓

01:30
主播下播

↓

Recording COMPLETED

↓

Songs Processing

↓

ACRCloud Recognition

↓

12 Songs Found

↓

自动生成音频
```

后台：

```text
Songs

12 Songs

晴天
泡沫
小幸运
如果可以
唯一
...
```

点击：

```text
▶ 晴天
```

播完：

```text
自动播放泡沫
```

识别错误：

```text
Reject
```

歌名错误：

```text
Edit
```

边界不准：

```text
修改 Start / End
→ Recut
```

正确：

```text
Confirm
```

---

# 59. V1 明确冻结范围

## V1 要做

```text
RecordingMediaTimeline

跨 FLV 音频切片

ACRCloud File Scanning

Audio Fingerprinting

Cover Song Identification

Music/Speech Evidence

Analysis Chunk

Result Normalizer

Result Aggregator

Song Draft

M4A Auto Cut

Songs Review UI

Single Song Player

Confirm / Reject

Recut

Global Songs Library
```

---

## V1 不做

```text
YAMNet

Whisper

Demucs

Humming Recognition

歌词识别

歌词搜索

弹幕辅助

Unknown Song AI Pipeline

网易云自动上传

自动 Vocal Separation
```

---

# 60. V2 演进方向

V1 稳定后：

```text
ACRCloud 没识别出来
        ↓
YAMNet
        ↓
检测 Singing
        ↓
Unknown Song
        ↓
Whisper
        ↓
歌词
        ↓
歌词搜索 / Humming
        ↓
song_candidates
```

再之后：

```text
弹幕
+
歌词
+
Cover
+
Fingerprint
+
Humming
```

可以组成真正的多证据识别系统。

---

# 61. 最终推荐架构

Songs V1 的核心可以归纳为：

```text
                     ┌──────────────────────┐
                     │ Recording COMPLETED  │
                     └──────────┬───────────┘
                                │
                                ▼
                 ┌────────────────────────────┐
                 │ RecordingMediaTimeline     │
                 │ ffprobe + discontinuity    │
                 └──────────────┬─────────────┘
                                │
                                ▼
                 ┌────────────────────────────┐
                 │ Analysis Audio             │
                 │ 2h Core + 30s Guard        │
                 │ mono / 32k / 64kbps MP3    │
                 └──────────────┬─────────────┘
                                │
                                ▼
           ┌────────────────────────────────────────┐
           │ ACRCloud Evidence Provider             │
           │ Fingerprint + Cover + Music/Speech    │
           └───────────────────┬────────────────────┘
                               │
                               ▼
                 ┌────────────────────────────┐
                 │ Recognition Evidence       │
                 │ Immutable raw matches      │
                 └──────────────┬─────────────┘
                                │
                                ▼
                 ┌────────────────────────────┐
                 │ Temporal Clustering        │
                 │ + Metadata Resolution      │
                 └──────────────┬─────────────┘
                                │
                                ▼
                 ┌────────────────────────────┐
                 │ Boundary Estimator         │
                 │ Music/Speech + fallback    │
                 └──────────────┬─────────────┘
                                │
                                ▼
                     ┌──────────────────┐
                     │ Song DRAFT       │
                     └────────┬─────────┘
                              │
                              ▼
                 ┌────────────────────────────┐
                 │ Source Retention Lease     │
                 │ + FFmpeg Clip Artifact     │
                 └──────────────┬─────────────┘
                                │
                                ▼
                     ┌──────────────────┐
                     │ Songs Web UI     │
                     │ Play / Confirm   │
                     │ Reject / Edit    │
                     │ Recut            │
                     └──────────────────┘
```

---

# 62. V1 Definition of Done

Songs V1 只有满足以下条件才算完成：

- [ ] 一场 Recording 可以包含多个 RecordingFile。
- [ ] Provider 原始识别结果以 Recognition Evidence 形式持久化。
- [ ] Aggregator 先按时间聚类，再解析歌曲身份。
- [ ] 聚合不得跨明显 discontinuity 自动 merge。
- [ ] Boundary Estimator 独立于 Provider Result。
- [ ] Analysis Chunk 使用明确的 Core + Guard 语义。
- [ ] Recording Cleanup 尊重 active Source Retention Lease。
- [ ] 可以正确建立统一 Recording Timeline。
- [ ] 任意跨 FLV 时间区间都可以正确切成一个 M4A。
- [ ] Recording 完成后可以自动启动 Songs Processing。
- [ ] 自动生成低码率 Analysis Chunks。
- [ ] 自动提交 ACRCloud。
- [ ] 可以在重启后继续 Poll 已提交扫描。
- [ ] 同时启用 Fingerprint + Cover Song Identification。
- [ ] ACRCloud 结果转换为统一 RawSongMatch。
- [ ] Analysis Chunk overlap 不会产生重复歌曲。
- [ ] Fingerprint 与 Cover 同曲结果可以聚合。
- [ ] 过短误识别能够被过滤。
- [ ] 自动生成 Song Draft。
- [ ] 自动从原始录像生成高质量 M4A。
- [ ] Songs 页面能够逐首播放。
- [ ] 支持 Previous / Next。
- [ ] 支持修改歌名 / 歌手。
- [ ] 支持修改 Start / End。
- [ ] 支持 Recut。
- [ ] 支持 Confirm。
- [ ] 支持 Reject。
- [ ] Songs 失败不会影响 Recording / Bilibili / COS。
- [ ] Songs 音频不会随着原录像滚动删除。
- [ ] Analysis 临时文件可以自动清理。
- [ ] 部分 Chunk 失败不会丢失已识别歌曲。
- [ ] 所有 Job 具备幂等性和重试能力。

---

# 63. 参考资料

ACRCloud Documentation:

- File Scanning:
  https://docs.acrcloud.com/reference/console-api/file-scanning/file-scanning

- Music Recognition / File Scanning Tutorial:
  https://docs.acrcloud.com/tutorials/recognize-music

- Cover Song Identification:
  https://www.acrcloud.com/cover-song-identification/

- Humming Recognition（V2 参考）:
  https://www.acrcloud.com/humming-recognition/

本文件只定义 7GRecorder Songs V1 的目标架构与实现边界。实际接入第三方 API 时，应再次以当时最新的 ACRCloud API 文档、字段定义、配额与价格为准。
