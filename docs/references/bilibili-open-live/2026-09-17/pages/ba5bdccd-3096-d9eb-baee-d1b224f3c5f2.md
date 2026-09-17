> 官方文档快照 · [在线原文](https://open-live.bilibili.com/document/ba5bdccd-3096-d9eb-baee-d1b224f3c5f2) · 抓取时间：2026-09-17T18:35:12.779127+00:00

# 互动面板

<font color=Red>注意</font>
仅互动玩法类型的项目支持互动面板。
未上架项目无法创建互动面板，请在项目上架后再申请，否则将会被驳回。

## 介绍

互动面板是用户快速参与互动玩法的一种方式，直播间开启玩法时互动面板出现在直播间里，用户点击面板上的指令可快速参与玩法。

![介绍](../assets/820924a7886126cd2b213c087169c881c3eceabb837797ea541052788511eb8f.png)

## 玩法介绍

描述本直播互动玩法的核心玩法，展示用户需要了解的玩法信息，可附加1张图片用以补充说明，文本部分最多600字。

<img style="width:507px" src="../assets/daec726bd42fdf3c28c577912dcf23ff08f6c821b44819d2eaf80b4e5fa5c64c.jpg"></img>

## 弹幕指令

用户通过互动面板弹幕指令点击一键发送弹幕。

![弹幕指令](../assets/466a07353a753c909fd230f0067a873ae1fcff4701e2ce464a4b2f41593e030f.png)


|   类型   |                     说明                     |
| :------: | :-------------------------------------------: |
| 弹幕指令 | 展示在互动面板上时的文字显示，最大显示4个字符 |
| 发送文案 |       用户发送的弹幕文案，不超过10个字       |
| 效果描述 |        该指令效果详细描述，最大20个字        |

示意图

<img style="width:442px" src="../assets/0652c831e0993b6daa5cfaaa729b356506772462f6e4b4a08ce998154c944eda.jpg"></img>



## 礼物指令

用户通过互动面板礼物指令点击一键发送礼物。

<img style="width:394px" src="../assets/02430ce84bfc97f3be758ec24dcda9e50c0883236ac7994ca065716fe0de42bf.png"></img>

在[创作者服务中心](https://open-live.bilibili.com/open-manage) ▶ 我的项目 ▶ 礼物管理内创建礼物并通过审核后，将自动与面板绑定并显示。

示意图

<img style="width:441px" src="../assets/5673c3b699d0172555fc31eff44dc53410e36dbc451c27f3d2445b6b6913862b.jpg"></img>


# 自定义互动面板

<font color=Red>注意</font>
仅特殊支持，合作申请live-open@bilibili.com

![](../assets/01259f1432a97d7ed82fc9ef9d0252e6feceffb5cb8b9bc02fe4676f69df4673.png)


## 前端规范

Iframe安全级别

参考文档：https://www.bookstack.cn/read/html-tutorial/spilt.2.docs-iframe.md

目前开放的级别：

```json
sandbox="allow-scripts allow-forms"
```

请开发者在服务端下发html资源时，配置如下响应头（key-value）：
```
Cross-Origin-Embedder-Policy：require-corp
```

响应头详情可参考[MDN文档说明](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Cross-Origin-Embedder-Policy)

## 封面图

说明：为玩法指令tab打开后，用户进入前默认展示的封面图

尺寸：375px * 195px
![](../assets/12bbb4e81c51b9becf3b6d6a050fb4af371174fc152ad3fcd35f3be686b43d5e.jpeg)