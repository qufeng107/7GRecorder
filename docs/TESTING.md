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
- Upload source merge jobs cover worker dispatch, FFmpeg adapter request construction, source transition to
  `PACKAGE_PENDING`, and terminal failure visibility.
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
- COS compression tests must verify that compression writes only to controlled temp/derived paths, never overwrites
  source output parts, validates the compressed file with ffprobe before upload, records source/uploaded sizes, and
  marks only the COS object failed when compression fails.
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
