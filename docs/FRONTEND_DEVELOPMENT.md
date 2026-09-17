# 前端本地开发与隔离测试

现有后台页面已按模块迁移：总览、录制配置、录像/审核、上传、歌曲、任务、系统、账号及个人信息。
共享登录、侧栏、明暗主题、中英文和移动导航；每个路由仅挂载本模块查询。
公开页面与控制台按需加载，`/@demo` 目前仍为占位页。

## 首次准备

固定 Node **22.18.0**、pnpm **10.15.0**；Linux x86_64 可在仓库根目录运行：

```bash
bash scripts/dev/setup-node.sh
```

脚本从 Node 官网下载到被 Git 忽略的 `.node-toolchain/`，验证 SHA-256，安装固定 pnpm，
按提交的 `frontend/pnpm-lock.yaml` 冻结安装。其他系统自行安装相同版本后使用 pnpm。
不升级当前 React/Vite 等依赖。真实后端联调另需 Go（仓库固定版本）、C 编译器及 Python 3。

## 纯前端模拟模式

```bash
bash scripts/dev/frontend.sh dev:mock
```

打开 **http://127.0.0.1:5173/admin/jobs**。

- 默认使用合成管理员与任务，无需登录真实账号。
- 页面右下角切换正常、空数据、错误、慢速加载、普通用户、无权访问、未登录场景。
- 模拟数据存在开发服务器内存中，刷新保留本次动作，切换场景重置；多个浏览器共享场景状态。
- 已实现接口由 Vite 开发中间件直接响应；未知 API 返回 `501 MOCK_NOT_IMPLEMENTED`，没有真实 API 代理。
- 合成数据覆盖录制配置、审核/剪辑、下载、上传配置、账号和清理等关键路径。歌曲等外部集成仅提供空数据；未知操作仍返回 501。
- fixture 中间件只在 `mock` 开发模式启用，不写入生产浏览器包。不要把模拟登录当作真实权限验证。

## 真实本地联调

```bash
bash scripts/dev/frontend.sh dev:integration
```

脚本构建本地 Go 后端，每次生成新的 `data/dev/run-*/` 数据库、密钥、媒体目录及合成账号，
打印本次的 `local-admin` 登录密码。前端为 5173，后端仅监听 `127.0.0.1:18080`。
Vite 的普通 `dev` 模式也固定代理到这个本地后端端口，不指向生产站点。

后端进程采用环境变量白名单，不继承 `~/.bashrc` 中的 B站/COS 等业务凭证；
Recorder 指向本机关闭端口，媒体和投稿工具设为不可用路径，数据库没有启用外部模块。
当前后端 `serve` 会运行 worker，因此不要向此测试实例添加真实凭证或导入生产数据。
这里提供的是独立数据与配置环境，不是网络沙箱；若测试外部集成，需另行添加对应 fake 服务。

## 构建产物隔离预览

```bash
bash scripts/dev/frontend.sh dev:preview
```

打开 **http://127.0.0.1:4173/admin/jobs**。它使用真实的前端生产构建和新建本地 Go 后端，
可验证静态构建、Cookie 登录和 API。仅供本机测试；Vite preview 不作为生产服务器。

两个联调模式都拒绝占用已有端口；用 Ctrl+C 停止其子进程。数据和 `backend.log` 留在对应 run 目录供排查，
脚本不会自动删除。仅清理确认不用的 `data/dev/run-*` 目录。不同运行的数据独立，但同一
`127.0.0.1` 主机的 Cookie 不按端口隔离；切换实例可使用新的浏览器上下文或重新登录。
生产域名及其 Cookie 不参与此环境。

## 检查与浏览器测试

```bash
bash scripts/dev/frontend.sh lint
bash scripts/dev/frontend.sh typecheck
bash scripts/dev/frontend.sh test
bash scripts/dev/frontend.sh build
bash scripts/dev/frontend.sh exec playwright install chromium
bash scripts/dev/frontend.sh test:e2e
bash scripts/dev/frontend.sh test:integration
```

- 单元/组件测试：保留旧页面测试，新增共享 API 错误及同源目标检查。
- 模拟浏览器测试：深链接、搜索 URL、主题记忆、任务状态变更、不确定投稿确认、场景切换、
  普通用户、拒绝访问、退出和会话过期、公开页面请求隔离、移动端布局与减弱动画模式。
- 真实联调浏览器测试：在新的数据库上验证登录/退出、所有后台页面、录制配置与上传模板持久化、空密码保留和普通用户权限。
- Playwright 固定 **1.55.0**；测试使用该版本下载的 Chromium，并自动启动/关闭本地服务。
  首次 Linux 安装若缺少浏览器系统库，执行 `pnpm exec playwright install --with-deps chromium`。
- `test-results/mock/` 与 `test-results/integration/` 保存失败 trace；测试账号为仅用于本地临时实例的合成账号。
- 生产部署和 `dev` 分支语义不变；CI 新增契约漂移和上述两类浏览器测试，部署安装改用冻结锁文件。

生成控制台资源类型：

```bash
bash scripts/dev/frontend.sh contracts
```

生成源是后端现有 JSON struct，输出 `frontend/src/shared/api/contracts.generated.ts`。
覆盖 Jobs/session、录制/上传源、存储、凭证元数据、账号、TLS 和歌曲的资源结构。
当前手工绑定 HTTP handlers 没有可直接导出的完整 OpenAPI；
列表/session envelope 仍由 typed wrapper 明确声明，不宣称全部接口已经自动生成。

## 当前边界与后续迭代

运行时不再引用旧 AdminDashboard；它仅作为既有回归测试的兼容入口。
本轮保留已部署的歌曲提供方与业务行为，没有数据库变更。创意公开页、动画、SEO 和新的直播运营分析仍待后续开发。
共享字段、表格与弹窗已使用控制台主题；更细的交互与视觉优化可以逐模块推进。

浏览器回归覆盖录像详情内的下载、应用剪辑不等于审核通过、审核失败提示、清理确认、受限路由不发业务请求，
以及录制/账号编辑不被轮询重置。表单跨路由离开不自动保存；设置表单仍按当前服务器响应刷新，后续可增加脏状态提示。
本轮“部署测试”默认指上述本地构建预览，远程测试域名与生产发布单独确定。
