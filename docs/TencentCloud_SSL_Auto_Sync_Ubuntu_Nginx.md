# 腾讯云 SSL 证书自动同步到 Ubuntu Nginx 方案

## 1. 目标

给腾讯云轻量应用服务器上的 Ubuntu + Nginx 自动更新 SSL 证书。

目标是：

- 证书继续由腾讯云 SSL 证书服务申请、续期。
- Ubuntu 服务器每天自动检查腾讯云是否产生了新的已签发证书。
- 如果有新证书，自动下载 Nginx 格式证书并替换本机证书。
- 自动执行 `nginx -t`，成功后 reload Nginx。
- 不使用 COS 中转。
- 不使用 Certbot / Let's Encrypt。
- 尽量简单、稳定，维护成本低。

---

## 2. 最终架构

```text
腾讯云 SSL 证书服务
        │
        │ 自动续期 / 新证书签发
        ▼
DescribeCertificates API
        │
        │ Ubuntu 每天检查一次
        ▼
发现新的 CertificateId
        │
        ▼
DescribeDownloadCertificateUrl API
        │
        ▼
下载 Nginx 格式证书 ZIP
        │
        ▼
解压出证书和私钥
        │
        ▼
覆盖本机 Nginx SSL 文件
        │
        ▼
nginx -t
        │
        ├── 失败：恢复旧证书
        │
        └── 成功：reload nginx
```

不需要：

```text
腾讯 SSL → COS → Ubuntu
```

直接：

```text
腾讯 SSL → Ubuntu
```

即可。

---

## 3. 腾讯云侧配置

### 3.1 证书

继续使用腾讯云 SSL 控制台里的证书。

例如：

```text
7g.chat
www.7g.chat
```

保证证书处于：

```text
已签发
```

并且后续能够正常自动续期/重新签发。

> 腾讯云续期后通常会产生一张新的证书，因此新的证书会有新的 `CertificateId`。

### 3.2 API 密钥

为了简单，不写复杂的自定义 CAM 最小权限策略。

推荐：

1. 创建一个 CAM 子用户，例如：

```text
ssl-sync
```

2. 给这个子用户关联腾讯云 SSL 证书服务的权限，例如：

```text
QcloudSSLFullAccess
```

3. 创建 API 密钥，得到：

```text
SecretId
SecretKey
```

不要使用主账号 API Key。

---

## 4. Ubuntu 目录规划

建议：

```text
/opt/tencent-ssl-sync/
├── update_ssl.py
└── current_cert_id

/etc/tencent-ssl-sync.env

/etc/nginx/ssl/7g.chat/
├── fullchain.pem
└── privkey.pem
```

Nginx 永远只使用固定路径：

```nginx
ssl_certificate     /etc/nginx/ssl/7g.chat/fullchain.pem;
ssl_certificate_key /etc/nginx/ssl/7g.chat/privkey.pem;
```

以后证书更新只覆盖这两个文件，不需要修改 Nginx 配置。

---

## 5. 安装依赖

Ubuntu：

```bash
sudo apt update
sudo apt install -y python3 python3-pip unzip openssl
```

安装腾讯云 Python SDK：

```bash
sudo pip3 install tencentcloud-sdk-python
```

如果系统禁止直接使用 `pip3` 安装全局包，也可以使用虚拟环境：

```bash
sudo apt install -y python3-venv

sudo mkdir -p /opt/tencent-ssl-sync
sudo python3 -m venv /opt/tencent-ssl-sync/venv

sudo /opt/tencent-ssl-sync/venv/bin/pip install tencentcloud-sdk-python
```

推荐使用虚拟环境，后续 systemd 直接调用：

```text
/opt/tencent-ssl-sync/venv/bin/python
```

---

## 6. API 密钥配置

创建：

```bash
sudo nano /etc/tencent-ssl-sync.env
```

内容：

```bash
TENCENTCLOUD_SECRET_ID=你的SecretId
TENCENTCLOUD_SECRET_KEY=你的SecretKey
SSL_DOMAIN=7g.chat
```

简单限制一下权限：

```bash
sudo chmod 600 /etc/tencent-ssl-sync.env
```

---

## 7. 同步脚本需要做什么

`update_ssl.py` 的逻辑保持简单：

### 第一步：查证书

调用：

```text
DescribeCertificates
```

查询：

```text
SearchKey = 7g.chat
CertificateStatus = [1]
CertificateType = SVR
```

其中：

```text
Status = 1
```

代表证书已经签发。

如果搜索结果有多张证书，只选择：

1. 域名确实包含 `7g.chat`
2. 状态为已签发
3. `CertEndTime` 最晚

的那一张。

---

### 第二步：判断是不是新证书

服务器保存当前已经部署的腾讯云证书 ID：

```text
/opt/tencent-ssl-sync/current_cert_id
```

例如：

```text
abc123456
```

如果腾讯云最新证书还是：

```text
abc123456
```

则：

```text
没有更新 → 直接退出
```

如果变成：

```text
xyz789012
```

则：

```text
发现新证书 → 开始下载
```

---

### 第三步：下载 Nginx 证书

调用：

```text
DescribeDownloadCertificateUrl
```

参数：

```text
CertificateId = 新 CertificateId
ServiceType = nginx
```

腾讯云返回一个临时 ZIP 下载地址。

脚本立即下载 ZIP 并解压。

不需要 COS。

---

### 第四步：找到证书文件

腾讯云 Nginx ZIP 内通常会包含：

```text
证书文件
私钥文件
```

脚本不要依赖固定文件名，而是根据内容识别：

私钥包含：

```text
-----BEGIN PRIVATE KEY-----
```

或：

```text
-----BEGIN RSA PRIVATE KEY-----
```

证书包含：

```text
-----BEGIN CERTIFICATE-----
```

如果存在 `bundle.crt` 一类的完整证书链文件，优先使用它作为：

```text
fullchain.pem
```

私钥保存为：

```text
privkey.pem
```

---

## 8. 替换证书

更新之前先保留当前文件：

```text
fullchain.pem.bak
privkey.pem.bak
```

然后写入新证书：

```text
/etc/nginx/ssl/7g.chat/fullchain.pem
/etc/nginx/ssl/7g.chat/privkey.pem
```

设置权限：

```bash
sudo chmod 644 /etc/nginx/ssl/7g.chat/fullchain.pem
sudo chmod 600 /etc/nginx/ssl/7g.chat/privkey.pem
```

---

## 9. 更新 Nginx

替换以后执行：

```bash
sudo nginx -t
```

如果成功：

```bash
sudo systemctl reload nginx
```

然后把新的：

```text
CertificateId
```

写入：

```text
/opt/tencent-ssl-sync/current_cert_id
```

整个更新完成。

---

## 10. 如果更新失败

只做一个简单的回滚即可。

如果：

```bash
nginx -t
```

失败，则把：

```text
fullchain.pem.bak
privkey.pem.bak
```

恢复回去。

然后再次：

```bash
nginx -t
```

保持原来的证书继续工作。

不要因为 SSL 更新脚本失败而停止 Nginx。

---

## 11. systemd 定时任务

不使用 cron，推荐 systemd timer，方便以后查看日志。

### Service

创建：

```bash
sudo nano /etc/systemd/system/tencent-ssl-sync.service
```

内容：

```ini
[Unit]
Description=Sync Tencent Cloud SSL certificate to Nginx
After=network-online.target

[Service]
Type=oneshot
EnvironmentFile=/etc/tencent-ssl-sync.env
ExecStart=/opt/tencent-ssl-sync/venv/bin/python /opt/tencent-ssl-sync/update_ssl.py
```

---

### Timer

创建：

```bash
sudo nano /etc/systemd/system/tencent-ssl-sync.timer
```

内容：

```ini
[Unit]
Description=Daily Tencent Cloud SSL certificate sync

[Timer]
OnCalendar=*-*-* 03:20:00
Persistent=true

[Install]
WantedBy=timers.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tencent-ssl-sync.timer
```

每天凌晨：

```text
03:20
```

检查一次即可。

证书更新不是实时业务，没有必要每小时检查。

---

## 12. 手动测试

第一次配置好以后先手动执行：

```bash
sudo systemctl start tencent-ssl-sync.service
```

查看结果：

```bash
sudo systemctl status tencent-ssl-sync.service
```

查看日志：

```bash
sudo journalctl -u tencent-ssl-sync.service -n 100 --no-pager
```

查看定时器：

```bash
systemctl list-timers | grep tencent-ssl-sync
```

---

## 13. 验证当前证书

查看本机文件里的证书：

```bash
openssl x509 \
  -in /etc/nginx/ssl/7g.chat/fullchain.pem \
  -noout \
  -subject \
  -issuer \
  -dates
```

也可以查看网站实际提供的证书：

```bash
echo | openssl s_client \
  -servername 7g.chat \
  -connect 7g.chat:443 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates
```

---

## 14. 第一次部署流程

建议第一次按这个顺序操作：

```text
1. 腾讯云 SSL 控制台确认 7g.chat 证书已经签发
        ↓
2. 确认证书可以正常下载
        ↓
3. 创建 CAM 子用户和 API SecretId / SecretKey
        ↓
4. Ubuntu 安装 Python SDK
        ↓
5. 创建 /etc/tencent-ssl-sync.env
        ↓
6. 创建 update_ssl.py
        ↓
7. 修改 Nginx，让证书固定指向：
   /etc/nginx/ssl/7g.chat/fullchain.pem
   /etc/nginx/ssl/7g.chat/privkey.pem
        ↓
8. 手动运行一次同步
        ↓
9. nginx -t
        ↓
10. 浏览器确认 HTTPS 正常
        ↓
11. 开启 systemd timer
```

---

## 15. 日常维护

正常情况下不需要人工处理。

服务器每天自动执行：

```text
检查腾讯云
    ↓
没有新证书
    ↓
结束
```

只有续期后才会：

```text
发现新 CertificateId
    ↓
下载
    ↓
替换
    ↓
nginx -t
    ↓
reload
```

以后主要只需要偶尔检查：

```bash
systemctl status tencent-ssl-sync.timer
```

以及：

```bash
journalctl -u tencent-ssl-sync.service
```

---

## 16. 方案取舍

最终不采用：

```text
Certbot + Let's Encrypt
```

也不采用：

```text
腾讯云 SSL
    ↓
COS
    ↓
Ubuntu
```

采用：

```text
腾讯云 SSL
    ↓
腾讯云 SSL API
    ↓
Ubuntu
    ↓
Nginx
```

优点：

- 链路最短。
- 不需要 COS。
- 不需要额外存储证书。
- 不依赖 Certbot。
- 腾讯云负责证书生命周期。
- Ubuntu 只负责部署。
- 一天检查一次即可。
- 出问题时旧证书仍可继续使用。
- 运维非常简单。

---

## 17. 一个需要提前确认的点

第一次上线前，手动确认当前腾讯云证书支持下载即可。

腾讯云 SSL API 的证书列表中存在 `AllowDownload` 字段；如果某张证书被设置为不可下载，则下载 API 无法获取它。

正常可下载的腾讯云 SSL 证书即可使用本方案。

如果未来腾讯云托管产生的新证书出现不可下载的情况，再针对该证书调整腾讯云侧配置即可，不需要改变 Ubuntu 整体架构。

---

# 最终推荐

对于：

```text
腾讯云轻量应用服务器
Ubuntu
Nginx
7g.chat
```

采用：

```text
腾讯云负责证书续期
+
Ubuntu 每天通过 SSL API 检查新证书
+
发现新证书后自动下载并 reload Nginx
```

这是当前最简单、稳定、容易维护的实现方式。
