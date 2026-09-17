# 互动面板

<font color=Red>注意</font>
仅互动玩法类型的项目支持互动面板。
未上架项目无法创建互动面板，请在项目上架后再申请，否则将会被驳回。

## 介绍

互动面板是用户快速参与互动玩法的一种方式，直播间开启玩法时互动面板出现在直播间里，用户点击面板上的指令可快速参与玩法。

![介绍](https://i0.hdslb.com/bfs/activity-plat/static/20230506/cc51945399520f91bb65629803b43743/introduce.png)

## 玩法介绍

描述本直播互动玩法的核心玩法，展示用户需要了解的玩法信息，可附加1张图片用以补充说明，文本部分最多600字。

<img style="width:507px" src="https://i0.hdslb.com/bfs/activity-plat/static/20240508/987dea895599a5c4df55fa2287156a94/nrpKb0xGcE.jpg"></img>

## 弹幕指令

用户通过互动面板弹幕指令点击一键发送弹幕。

![弹幕指令](https://i0.hdslb.com/bfs/activity-plat/static/20230506/cc51945399520f91bb65629803b43743/dm.png)


|   类型   |                     说明                     |
| :------: | :-------------------------------------------: |
| 弹幕指令 | 展示在互动面板上时的文字显示，最大显示4个字符 |
| 发送文案 |       用户发送的弹幕文案，不超过10个字       |
| 效果描述 |        该指令效果详细描述，最大20个字        |

示意图

<img style="width:442px" src="https://i0.hdslb.com/bfs/activity-plat/static/20240508/987dea895599a5c4df55fa2287156a94/eBsRyHEd8s.jpg"></img>



## 礼物指令

用户通过互动面板礼物指令点击一键发送礼物。

<img style="width:394px" src="https://i0.hdslb.com/bfs/activity-plat/static/20230506/cc51945399520f91bb65629803b43743/gift.png"></img>

在[创作者服务中心](https://open-live.bilibili.com/open-manage) ▶ 我的项目 ▶ 礼物管理内创建礼物并通过审核后，将自动与面板绑定并显示。

示意图

<img style="width:441px" src="https://i0.hdslb.com/bfs/activity-plat/static/20240508/987dea895599a5c4df55fa2287156a94/YDRLlacilp.jpg"></img>


# 自定义互动面板

<font color=Red>注意</font>
仅特殊支持，合作申请live-open@bilibili.com

![](http://i0.hdslb.com/bfs/templar/open-static/2862c3681052b83317d12ff2996bd87a3f8ccbaf.png#size=453x526&align=left)


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
![](http://i0.hdslb.com/bfs/activity-plat/static/20240611/ded97441444adb3f0b32e68add9f2708/vpZmV9AxA3.jpeg#size=395x235&align=left)