# 7GRecorder — 测试规范

## 1. 原则

优先测试可能造成真实损失或跨模块污染的路径：

1. 不能错误结束/串联 Recording；
2. 本地滚动存储不能写满服务器；
3. 不能删除 active/writing/in-use/protected 文件；
4. Bilibili 不能重复投稿；
5. COS 不能误删 Bucket 中非 7GRecorder 对象；
6. 一个可选模块失败不能改变其他模块状态；
7. Manager 不能越权；
8. Secret 不能泄漏；
9. 重启后 Job/Recording 能恢复。

---

## 2. Unit

重点：

- grace period；
- Session merge；
- Local rolling candidate selection；
- local quota / free-space threshold；
- COS rolling candidate selection；
- Job backoff/idempotency；
- Publication ambiguous rules；
- ownership/policy；
- template rendering。

---

## 3. Integration

使用临时 SQLite + 临时目录：

- migration；
- Job atomic claim；
- event idempotency；
- Profile ownership；
- local usage calculation；
- rolling deletion transaction + filesystem result；
- in-use Job prevents local delete；
- COS object metadata transitions；
- credential encryption round-trip。

---

## 4. Adapter

Fake/controlled external boundaries：

- BililiveRecorder HTTP；
- webhook payload parsing；
- biliup command/result parsing；
- FFmpeg command；
- COS client Adapter；
- NetEase/Publisher Adapter。

普通 CI 不依赖真实 Bilibili/COS/网易云账号。

Bilibili CLI adapter tests use a fake executable instead of a real Bilibili account. The test verifies argument
construction, temp `cookies.json` creation, BV id parsing, and rejection of browser cookie strings. Real biliup fixture
refresh remains a pinned-version maintenance task and must not introduce real secrets into the repository.

The fake biliup test must also assert that multipart command paths use temporary `p01`, `p02`, ... aliases, that each
alias resolves to the original source, and that canonical local/COS-facing filenames are not renamed.

Progress regression tests include carriage-return progress lines and ANSI control sequences from the pinned biliup
renderer, and assert non-zero aggregate byte progress.

Worker heartbeat regression tests verify that a long-running claimed Job refreshes `heartbeat_at` even when its
adapter emits no progress, that heartbeat-only updates leave `progress_updated_at` unchanged, and that a Worker cannot
refresh a Job owned by another lock identity. Frontend Job tests distinguish fresh-heartbeat/stale-progress from a
stale Worker heartbeat and must not label the former as stalled.

Upload Source discovery regression tests must prove that stale Profile runtime values (`LIVE`/`RECORDING`) cannot
permanently hide completed closed recordings. Existing adjacent `ACTIVE` Recording and `WRITING`/recent recorder-file
tests remain the safety boundary that prevents premature parent creation during an actual recording.
The adjacent-recording guard must also wait when the next segment starts at the exact previous completion second or
overlaps that completion by a small file-timestamp skew; both indexed and not-yet-indexed active files need coverage.
Recorder adapter and worker tests also cover read-only runtime polling from stale `LIVE/RECORDING` to `OFFLINE/IDLE`.

COS tests must verify that each object uploads the original publish-part path and bytes without invoking FFmpeg or
creating a derived file. Object-key tests must assert `videos/YYYY-MM-DD/session-NN/pNN.<source-format>`, China-date
daily ordinals, stable output ordering, and unchanged historical object rows. Bilibili must continue to read the same
original source part independently.

---

## 5. 模块隔离测试 — 必须有

### Bilibili Disabled

```text
Recording COMPLETED
Bilibili config absent
→ no Bilibili Job
→ Local Storage continues rolling
→ COS/Songs unaffected
```

### Bilibili Failed

```text
Upload FAILED
→ Recording remains COMPLETED
→ COS can become AVAILABLE
→ Local may later roll to DELETED
```

### COS Disabled

```text
File CLOSED
no COS profile
→ no COS Job
→ Recording/Bilibili unaffected
```

### COS Failed

```text
COS upload FAILED
→ Local remains independent
→ Bilibili can VERIFIED
```

### Local Rolled Before Optional Module

```text
Recording COMPLETED
Local cleanup deletes source
→ Publication/COS/Songs mark SOURCE_MISSING when they later inspect
→ no cross-module rollback
```

### Songs Failed

```text
song processing FAILED
→ Publication status unchanged
→ COS status unchanged
→ Recording remains COMPLETED
```

### Manual COS Songs V1

- 只有 AVAILABLE upload-source COS 视频可选，弹幕、失败、删除、替换和越权对象必须拒绝；
- 同一点击只创建一个 Run，所有阶段使用确定性 business key；
- COS 下载覆盖取消、进度、大小/ETag 校验和 `.part` 原子提升；
- 固定版本本地检测器 fixture 覆盖 speech/singing/music/silence、malformed output、timeout、cancel 和模型版本不匹配；
- 窗口/chunk 重叠去重、阈值滞回、短间隔合并、最短时长、padding、output-local 到 parent timeline 转换和
  边界 clamp 必须有测试；
- 人工标注样本必须验证高召回目标与审核时间收益，不能只验证模型命令成功退出；
- MobileNetV2/Cnn6 离线对比必须记录模型与完整 CPU runtime 磁盘占用、batch size 1 峰值内存和每小时音频
  处理耗时；轻量候选未达标前不引入 Cnn14/CUDA；
- M4A 从原视频生成并验证后上传 COS，不能从低码率分析 MP3 生成；
- 边界修改递增 revision、生成新版 M4A，并使旧音频与视频缓存失效；
- 播放 miss 合并为一个 Job，命中刷新 LRU，internal redirect 不泄露路径或 COS Secret；
- 视频只在手工请求后准确重编码，相同 revision 重用缓存且不自动上传 COS；
- 音频缓存不超过总上限 5%，视频导出缓存不超过 5GB；
- 并发空间预留不能超卖；租约、播放 grace、活动/受保护/最新录像不能被清理；
- 无安全空间时 Job 延后等待，不占用 Worker slot 忙等；
- 活动 Run/Export 的源 COS lease 阻止受控删除，外部删除转换为 SOURCE_MISSING；
- Songs 失败不得改变 Recording、Bilibili 和源 COS 成功状态。

当前 MVP 自动化覆盖：ACRCloud multipart 流式提交、Bearer 鉴权、ready/auth 响应、music 区间解析与相邻
同曲合并；COS 下载到独立 AI Job 的交接；result/evidence/Song/Artifact/cache 持久化；版本化 COS Key；
后台歌曲列表与音频元素。Worker 全链路使用 fake Recognizer/AudioCutter/COSUploader，在 Linux CI 执行。

上述 ACRCloud 覆盖属于已部署旧 checkpoint，保留为回归保护但不再是新 V1 验收门槛。下一 checkpoint 必须新增
本地检测器与高召回区间聚合测试，且证明未配置任何外部识别凭证时仍可完成候选 M4A 闭环。

---

## 6. Recording 高优先级场景

- 重复 Webhook 不产生重复 Session/File；
- 短断流归并；
- grace timeout finalize；
- FileClosed 无 FileOpening 仍能 UPSERT；
- server restart reconciliation；
- 多 Profile 不串数据；
- Stream LIVE 但 AutoRecord=false 不创建 Recording。

---

## 7. Local Storage 高优先级场景

- quota 未达到不删除；
- quota 达到按最旧 completed 删除；
- active Recording 永不普通滚动删除；
- WRITING File 永不删除；
- RUNNING Job 使用中的 File 跳过；
- protected Recording 跳过；
- 删除失败可重试且 metadata 不误标成功；
- Bilibili/COS/Songs 状态不参与普通本地滚动 eligibility；
- hard free-space floor 能阻止新重任务/新录制；
- storage metrics 与真实文件一致。

---

## 7.1 Live operations analytics

- 签名测试固定请求体、时间和 nonce，验证 MD5、规范化头和 HMAC 结果；
- Proto 测试覆盖 Version 0、多包 Version 2 zlib、截断包、未知版本和鉴权回复；
- Start/heartbeat/end 使用脱敏 HTTP fixture，业务 `code != 0` 即使 HTTP 200 也必须失败；
- 房间号与 Profile 不一致时拒绝采集并调用 end；
- 原始 writer 保存已知与未知 CMD，路径不能逃逸 `live-analytics` root；
- 会话计数、最后事件时间、未知事件和缺口批量落库；重复启动不能为同一 Profile 创建两个活动会话；
- 原始数据超过 2 GiB 子配额时只按时间删除已关闭分片；当前写入分片受保护，删除后大小与 metadata 仍可解释；
- 重启把遗留活动会话标为 `INTERRUPTED`，禁用/正常停止形成 `ENDED`；
- 采集失败不得创建或修改 Recording、Publication、COS、Song 或 Job 状态；
- API 覆盖 SUPER_ADMIN 写入、Manager ownership 读取和凭证明文不回显。

真实平台验收只使用用户授权测试房间并人工触发事件；fixture 必须脱敏，CI 不访问 Bilibili。

## 8. COS 高优先级场景

- closed segment 创建唯一上传任务；
- duplicate reconcile 不重复对象；
- quota 计算只包含 AVAILABLE managed objects；
- quota exceeded 删除最旧 managed Recording；
- 只删除 configured prefix + DB registered object；
- 未知 Bucket Object 永不删除；
- local source missing → SOURCE_MISSING；
- COS delete 不改变 local/Bilibili/Songs 状态；
- signed URL 不泄露长期 Credential；
- Local download 相对路径不能 path traversal/symlink escape；
- Nginx internal 下载不暴露真实宿主机路径。

---

## 9. Publication

- biliup CLI failure 不标成功；
- Bilibili title/description templates render into the upload request snapshot before the uploader adapter runs;
- upload process crash → AMBIGUOUS；
- AMBIGUOUS 不盲目重新投稿；
- verify 可恢复 external_id；
- local source missing → SOURCE_MISSING；
- retry business_key 不创建第二个 Publication。

---

## 10. Job

- business_key 唯一；
- stale LIGHT/MEDIA Job 可恢复；
- external side effect Job stale 走模块-specific ambiguous policy；
- Worker restart recovers orphaned COS/local jobs to `PENDING` without consuming an attempt, while orphaned Bilibili
  uploads become `AMBIGUOUS` and are never claimed automatically;
- an orphaned Bilibili job with an already `VERIFIED` Publication is finalized as `SUCCEEDED` without resubmission;
- ambiguous Bilibili retry requires explicit operator confirmation and atomically resets the existing Publication/Job;
- `worker_drain=true` prevents claims without mutating queued or running jobs;
- worker startup requeues completed Recorder sync jobs so persisted room settings are verified and corrected;
- resource class 并发限制；
- live recording 时不启动新的 NETWORK/MEDIA/AI；
- Storage Critical 可以阻止低优先级重任务。
- Admin Jobs API covers list visibility, failed/cancelled retry reset, and invalid running-job cancel rejection.
- Admin Jobs UI covers list rendering and retry action wiring.
- Recording group diagnostics cover adjacent same-profile grouping, real-gap splitting, short-segment flags, and
  read-only behavior before FFmpeg merge jobs exist.
- Upload source discovery covers the shared merge gap threshold, waiting until a profile is no longer recording,
  idempotent source creation, single-segment `PACKAGE_PENDING`, multi-segment `MERGE_PENDING`, backfilled missing merge
  and package jobs, and segment timeline metadata.
- Upload source discovery must wait instead of finalizing a parent source when a later same-profile recording starts
  inside the merge gap and is still `ACTIVE` or has a `WRITING` video file.
- Upload source discovery filesystem fallback must inspect only `DATA_ROOT/recordings`, delay for fresh same-room video
  files whose parsed start time is inside the merge gap, and ignore derived files under upload/processing directories.
- Upload source merge jobs cover worker dispatch, direct segment-to-output packaging, source transition to
  `READY_TO_UPLOAD`, upload-facing part names, timeline metadata, and terminal failure visibility.
- Segment packaging must prove that dimensions-only PK transitions use one normalized duration-based timeline with
  aspect-ratio-preserving black padding and no enlargement of smaller inputs. Other incompatible FFprobe signatures
  must remain separate stream-copy groups, and normalized FFmpeg failure must fall back to those ordered safe groups.
- Upload source package jobs cover worker dispatch, output part persistence, source transition to `READY_TO_UPLOAD`,
  post-package timeline metadata, upload-facing part names, and China-time live ordinals.
- Upload module reconciliation covers credential secret encryption, disabled-module no-op behavior, `READY_TO_UPLOAD`
  source detection, idempotent Bilibili publication/job creation, idempotent COS object/job creation, and COS object
  keys that follow post-package part paths.
- Upload module reconciliation must not create Bilibili/COS jobs for a `READY_TO_UPLOAD` source that has a later
  same-profile adjacent recording outside the source inside the merge gap.
- Upload source regroup tests must verify that fragmented historical sources can be explicitly replaced without
  deleting original recordings or COS objects, that `REPLACED` sources disappear from normal lists, and that groups with
  Bilibili publications or running upload-source jobs are blocked.
- Upload source repair tests must verify DB/filesystem drift recovery: missing derived merge/package files roll a source
  back to the correct upstream state when original segment files still exist; old succeeded/failed/cancelled merge and
  package jobs are reset safely; downstream upload jobs are cancelled until the source is ready again; package reruns
  update existing output rows without violating COS object foreign keys; missing original segment files block repair
  instead of creating partial uploads.
- COS upload jobs cover worker dispatch with a fake COS uploader, encrypted credential decryption, source path
  resolution under `DATA_ROOT`, object transition to `AVAILABLE`, and job success. Real COS credentials are not used in
  CI.
- COS download tests must verify that only `AVAILABLE` upload source output objects can produce signed download
  requests, that profile visibility and manager policy are enforced before signing, and that old failed COS jobs do not
  make the current output summary look failed after a later successful upload.
- COS direct-upload tests must verify that no FFmpeg/archive derivative is created, the current output path and bytes are
  passed directly to the COS adapter, and historical terminal object rows are not rewritten by reconciliation.
- Raw danmaku archive tests must verify scan attachment to the matching recording, idempotent `cos_objects` and
  `UPLOAD_COS_RECORDING_FILE` job creation, worker upload status transitions, and signed URL generation only after the
  raw COS object is `AVAILABLE`.
- Disk housekeeping tests must verify deploy/daily cleanup removes only whitelisted release artifacts, stale build
  cache, old DB backups, and safe temp leftovers; it must not delete original recordings, active upload-source files,
  BililiveRecorder workdir/images, SQLite WAL/SHM files, or unknown COS objects.

---

## 11. Authorization / Credentials

- MANAGER 只能访问自己的 Profiles/Recordings/Jobs/Storage module config；
- SUPER_ADMIN 可跨 Profile；
- ManagerPolicy 正确限制修改能力；
- DB 无 plaintext Secret；
- API 不回显 Secret；
- Logs 无 Secret；
- master key 缺失时 fail closed。

---

## 12. Frontend

前端模块化迭代新增目标：合成数据开发模式、隔离本地 API 联调、按模块组件测试和 Playwright 浏览器 smoke。
覆盖独立路由直达/刷新、账号权限、空/错误/加载状态、编辑与审核分离、清理及重复投稿确认。
公开主播页验证移动端、键盘访问、减弱动画和后台/动画资源加载隔离。
测试不使用生产凭证或文件，mock 不可向生产透传；UI mock 不能替代 API 契约和联调验证。
具体实施顺序见 `FRONTEND_UI.md`；Jobs、全部控制台路由、会话与公开路由隔离的浏览器测试及本地隔离环境已实现，
运行方法见 `FRONTEND_DEVELOPMENT.md`。现有模块的审核、清理、权限、设置及编辑草稿均已加入浏览器回归。

保持精简：

- TypeScript typecheck；
- 权限/模块 enabled 状态；
- Recording 三来源 Local/COS/Bilibili 展示；
- Storage quota/critical UI；
- API Contract；
- 主要页面 smoke。

不做大量低价值 snapshot test。

---

## 13. E2E Smoke

至少覆盖：

```text
login
→ create profile
→ recorder webhook
→ Recording completed
→ local storage visible
→ Bilibili disabled means no job
→ enable Bilibili + fake uploader
→ verified
→ enable COS + fake object store
→ available
→ shrink local quota
→ oldest local recording deleted
→ Bilibili/COS state unchanged
```

---

## 14. Deployment / Recovery

至少覆盖脚本级/Smoke 行为：

- `dev` workflow 只执行 CI，不执行 Production Deploy；
- `main` Deploy 必须依赖 CI 成功；
- Release SHA/checksum 校验失败时不得安装；
- migration 前创建可打开的 SQLite backup；
- Backend 发布只重启 7GRecorder，不执行 `docker compose down`；
- deploy drain prevents new claims and refuses to recreate the current container while it owns any `RUNNING` job;
- deploy exit paths clear the temporary drain control;
- 模拟 Backend restart 时 BililiveRecorder 继续运行；
- Backend ready 后 deploy 才成功；
- 回滚到上一 Git SHA 后 Backend/Frontend 版本一致；
- 上一版应用可在新 migration schema 上启动（one-release backward compatibility）；
- `/internal/*` 不经公网 Nginx 暴露；
- Production Secret 不存在于 release artifact；
- old release/image cleanup 不触碰 `/data/7grecorder`。
- deploy/daily disk housekeeping covers stale Docker build cache and bounded backup/temp cleanup without touching
  business media outside approved guards.

---

## 15. Release Gate

Site TLS coverage:

- only SUPER_ADMIN can read, update, or manually sync site TLS settings;
- only a `SYSTEM / tencent_ssl / TLS` credential can enable the module;
- API secret plaintext and certificate private keys are never returned;
- certificate selection requires exact X.509 hostname coverage for `7g.chat` and `www.7g.chat`;
- malformed ZIPs, unsafe paths, mismatched keys, and expired certificates are rejected without replacing staged data;
- `SYNC_SITE_TLS` is durable, idempotent, restart-retryable, and isolated from Recording/upload jobs;
- the host installer keeps the previous certificate when host validation or `nginx -t` fails;
- Nginx redirects named HTTP hosts, serves both production names over TLS, and does not expose `/internal/*`.

Manual local cleanup coverage:

- cleanup runs only when local storage policy reports reclaim is needed;
- cleanup deletes closed video files for whole completed unprotected recordings;
- protected, active, writing, and RUNNING-job-referenced recordings are skipped;
- metadata is retained and marked `DELETED` after local files are removed.

```text
Backend:
  gofmt / vet
  unit + integration + adapter tests
  build
  migration from clean DB

Frontend:
  lint
  typecheck
  tests
  vite build

System:
  compose config/smoke
  API contract generation/type consistency
  SQLite backup/restore smoke when DB changes
  local rolling-storage safety smoke when storage logic changes
  production release checksum/preflight script tests
```

`main` Production Deploy 只有在上述 CI gate 成功后执行；`dev` 永不部署正式服务器。

不得通过放宽/跳过测试来完成 release。
# 2026-09-11 review gate tests

- Upload reconcile tests must cover `upload_sources.review_status = 'REQUIRED'` and assert that Bilibili publications/jobs and upload-source COS objects/jobs are not created.
- Worker request loaders for Bilibili and upload-source COS must reject reviewed upload sources even if an older pending job exists.
- Recording store tests must cover local packaged-output download being available only for reviewed upload sources and only when the local file still exists.
- Worker tests must cover `APPLY_UPLOAD_SOURCE_EDIT` producing edited outputs, clearing `edit_decision_json`, and keeping the upload source blocked until review approval.
- Review tests must cover freezing running Bilibili/COS jobs, resetting their remote state, cancelling the worker context, and ignoring late worker completion after the persisted job is no longer `RUNNING`.
- Frontend checks should cover active recordings merged into the recording list once UI tests are expanded.

## Review/edit release gate

- Store tests must reject approval while `edit_decision_json` is non-empty.
- Store tests must prove approval resumes enabled destinations but leaves disabled Bilibili/COS records and jobs frozen; disabled destinations must never become runnable and fail as missing resources.
- Media tests must cover deletion at the beginning, middle, and end; multiple normalized ranges; ranges crossing a
  two-hour part boundary; all-content deletion rejection; cancellation; and missing input files.
- Successful edit tests must assert `parts/...` changes to `edited/...`, total duration equals original duration minus
  deleted duration, output timelines are contiguous, and `review_status` stays `REQUIRED`.
- Bilibili request tests must assert that only the current edited output paths are passed to biliup after approval.
- COS request tests must assert that direct-upload input and regenerated object keys use the current edited output rows.
- Invariant tests must attempt late publication/COS success after a review freeze and assert the database trigger rejects
  it.
- Frontend tests must distinguish `editing`, `waiting for review`, and `approved/uploading`, and must not treat a
  successful apply-edit HTTP response as completed media work.
- Manual acceptance: enter `00:00:00-00:19:53` against a `05:42:30` source and verify the displayed result is
  `05:22:37`, paths are `edited/...`, remote states still say waiting for review, and upload begins only after approval.

## Automatic upload-source cleanup tests

- No disk pressure means no files or metadata change.
- The oldest eligible source is reclaimed under pressure; its raw videos and controlled derived directory are
  deleted while publication/COS rows remain.
- The newest source per profile is retained even when fully delivered.
- Active/writing, protected, reviewed, editing and running-job sources are excluded.
- Remote success is not required; failed/pending/disabled remote destinations do not block cleanup.
- Cleanup persists `DELETING` before file removal and ends in `DELETED` or `FAILED`; repair skips all non-`AVAILABLE`
  cleanup states.


Frontend migration regression gate additionally covers:

- Profile creation/editing and draft retention during polling.
- Review downloads only in expanded output details; applying edits never approves delivery.
- Failed review requests surface an error; cleanup requests require explicit confirmation.
- Empty account passwords are omitted and preserve authentication against the real backend.
- Restricted deep links never mount module queries; real backend permissions still reject unauthorized requests.
- Built frontend renders every existing module with real API nullability and persists disabled upload configuration.

System-settings draft tests cover polling during edits, failed saves, edits during pending saves, latest-server
discard, independent TLS/storage state, cancelled/confirmed route departure, and beforeunload registration.
Secrets must not be persisted in browser storage. Run these locally without production credentials.

Completion regression tests additionally cover independent Bilibili/COS drafts, profile switching confirmation,
submission target stability, pending-save edits, song save failure/recovery, profile/account Escape confirmation,
unapplied review cuts blocking approval, credential clearing, and every console module at a 390px dark viewport.
Real-backend integration verifies disabled COS only persists the disabled state (other edits remain dirty), plus
song prefix normalization and reload persistence. Synthetic fixtures must match these API semantics.

OpenLive evidence browser regression: ownership before filesystem access, cursor boundary/limit validation,
bounded record reads, deleted evidence 410, malformed/partial JSONL reported without exposing host paths.

Rolling cleanup regression cases include failed/pending/disabled/absent upload modules, queued versus running jobs, recording/file job references, protected/writing inputs, and review gates. Reclaimed upload inputs must report `SOURCE_MISSING` before invoking external upload adapters.

原文分片测试：同一会话轮换保持事件顺序；活动会话的已关闭分片可清理，当前分片不删除；
原文件迁移、清理中断恢复、分片越权/跨会话读取、前端切换分片时重置分页游标。
