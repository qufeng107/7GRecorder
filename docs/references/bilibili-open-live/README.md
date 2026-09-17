# Bilibili 开放平台：直播与互玩文档参考

最新本地快照：[2026-09-17 官方发布目录](2026-09-17/README.md)。

本目录保存第三方参考资料，不替代 7GRecorder 的权威设计或真实脱敏 fixture。
版权及原始授权归官方/原作者，不随本仓库许可证变更。

## 常用入口

- [快速开始](2026-09-17/pages/849b924b-b421-8586-3e5e-765a72ec3840.md)
- [常见问题：AppId、身份码、open_id、调试](2026-09-17/pages/5dffc297-6fd2-41ff-bd45-6e8b89e2a68e.md)
- [统一鉴权与错误码](2026-09-17/pages/74eec767-e594-7ddd-6aba-257e8317c05d.md)
- [应用 API 列表](2026-09-17/pages/eba8e2e1-847d-e908-2e5c-7a1ec7d9266f.md)
- [长链数据协议](2026-09-17/pages/657d8e34-f926-a133-16c0-300c1afc6e6b.md)
- [长链命令说明](2026-09-17/pages/f9ce25be-312e-1f4a-85fd-fef21f1637f8.md)
- [Golang 示例入口](2026-09-17/pages/90512caa-056e-f78a-4b61-098a18301724.md)

## 读取与归档方式

普通 HTTP 读取页面只能得到 JavaScript 应用外壳。此次使用 Python Playwright 启动无登录 Chrome，
执行脚本后读取渲染正文，并观察到网站获取目录的公开请求：

`https://member.bilibili.com/arcopen/user/open-doc/view?id=13`

归档工具只处理响应的 `online_doc` 已发布目录，不处理 `draft_doc`；
沿目录提供的 Markdown URL 获取原文，并归档正文图片；不改写或抓取代码块中的示例地址。CDN 查询签名不写入抓取清单。
没有读取浏览器个人资料，也没有使用 Cookie、Access Key 或主播身份码。

正文原文在 `originals/`，本地可读版本在 `pages/`，图片在 `assets/`。
可读版仅添加来源提示、本地化文章及图片链接；原文不改写。外部 SDK、安装包、PDF 和演示视频仍为在线链接，
本快照不是这些外部下载资源的完整镜像。Markdown 中的 HTML 元素由所用阅读器决定是否支持。
原文中没有公开源地址的相对资源（如视频封面 `./img/introduce.png`）未归档；
清单中的 `complete` 只表示正文及识别到的代码块以外的公开绝对地址图片已抓取。

当前快照包含 40 篇文档、42 个图片资源，约 6.2 MB；文件哈希、代码块原样保留和目录链接已校验。

## 手动更新

使用 Python 3 标准库即可更新，无需给项目新增 Playwright 或其他生产依赖。
在仓库根目录执行，输出目录必须不存在，已有快照不会覆盖：

```bash
python3 scripts/docs/archive_bilibili_open_live.py \
  --output docs/references/bilibili-open-live/YYYY-MM-DD
```

更新后检查 `manifest.json` 的 `complete` 和 `asset_failures`、文章数量及 SHA-256，再更新本页链接。
工具失败时保留未完成目录供排查；未产生完整清单的目录不能标为成功快照。
官方文档变化仅更新参考资料，不自动升级 SDK、不改变生产配置或接入能力。
