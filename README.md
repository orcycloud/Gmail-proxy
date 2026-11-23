# Gmail Proxy

一个轻量、零依赖的 **TCP + TLS 透传代理**，专门解决这种场景：

> 邮箱客户端需要访问 `pop.gmail.com:995` 和 `smtp.gmail.com:465`，但客户端所在的电脑**没有外网权限**。
> 在一台能访问外网的服务器上运行本程序，客户端填写 `pop.localdomain.com` / `smtp.localdomain.com` 即可收发 Gmail。

服务器自己也**没有外网直连、需要再走一层 HTTP 代理**才能上网的情况，同样支持。

---

## 架构

### 场景 A：服务器能直连外网

```
┌───────────────┐    TCP    ┌────────────────────┐   TLS 透传   ┌─────────────────┐
│  邮件客户端   │──────────►│   Gmail Proxy      │────────────►│ pop.gmail.com   │
│ (无外网权限)   │ :995/465 │   :995  :465       │ :995  :465  │ smtp.gmail.com  │
└───────────────┘           └────────────────────┘             └─────────────────┘
                                代理服务器（有外网）
```

### 场景 B：服务器也需要出站代理

```
┌───────────────┐    TCP    ┌────────────────────┐  HTTP CONNECT  ┌──────────────┐   TLS 透传   ┌─────────────────┐
│  邮件客户端   │──────────►│   Gmail Proxy      │───────────────►│ HTTP 代理     │────────────►│ pop.gmail.com   │
│ (无外网权限)   │ :995/465 │   :995  :465       │   :10808       │ 10.0.0.1     │             │ smtp.gmail.com  │
└───────────────┘           └────────────────────┘                └──────────────┘             └─────────────────┘
```

**关键点：**

- 代理**不解密 TLS**，只是做 TCP 字节搬运。
- 客户端直接和 Gmail 完成 TLS 握手，Gmail 证书校验完全不受影响，也不需要代理持有任何证书。
- 纯 Go 标准库实现，零外部依赖。

---

## 目录结构

```
gmail-proxy/
├── go.mod
├── cmd/proxy/
│   ├── main.go         # 入口 + 信号处理
│   └── config.go       # YAML 配置解析（手写，无依赖）
├── internal/proxy/
│   └── tunnel.go       # TCP 转发核心 + HTTP CONNECT 实现
├── configs/config.yaml # 配置文件（含两套场景示例）
├── Dockerfile
├── docker-compose.yml
└── scripts/
    ├── build.ps1            # 本地 / 交叉编译
    └── install-windows.ps1  # 注册为 Windows 服务
```

---

## 配置

所有路由规则写在 [`configs/config.yaml`](configs/config.yaml)：

```yaml
tunnels:
  - name: pop3s
    listen: ":995"                         # 本地监听地址
    upstream: "pop.gmail.com:995"          # 转发目标
    timeout: 5m                            # 连接空闲超时
    # outbound_proxy: "10.0.0.1:10808"     # 出站 HTTP 代理（可选）
    # outbound_auth:  "user:pass"         # 代理认证（可选）

  - name: smtps
    listen: ":465"
    upstream: "smtp.gmail.com:465"
    timeout: 5m
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✓ | 日志里的别名，随便写 |
| `listen` | string | ✓ | 本地监听地址，如 `:995` 或 `0.0.0.0:995` |
| `upstream` | string | ✓ | 转发目标 `host:port` |
| `timeout` | duration | ✗ | 单连接空闲超时，默认 `5m`，格式参考 Go duration（`30s` / `1h`） |
| `outbound_proxy` | string | ✗ | 出站 HTTP 代理 `ip:port`，留空走直连 |
| `outbound_auth` | string | ✗ | 代理 Basic 认证，格式 `user:pass` |

**扩展 IMAPS（993）只需加一段：**

```yaml
  - name: imaps
    listen: ":993"
    upstream: "imap.gmail.com:993"
```

---

## 部署

### 方式一：Docker（推荐）

最简单的方式，宿主机直接暴露 995 / 465 端口。

```bash
git clone <your-repo> && cd gmail-proxy

# 编辑 configs/config.yaml，填入你的出站代理地址（如果需要）

docker compose up -d
```

日志查看：

```bash
docker logs -f gmail-proxy
```

> 默认用了 `network_mode: host`，容器直接用宿主机网络。
> 如果你在 NAS 或云主机上不方便用 host 网络，把 `network_mode` 注释掉，改用下面的端口映射：
>
> ```yaml
> ports:
>   - "995:995/tcp"
>   - "465:465/tcp"
> ```

### 方式二：Windows 原生服务

适合 Windows Server。已内置 PowerShell 脚本，用系统自带的 `sc.exe` 注册，不需要额外安装 NSSM 等工具。

```powershell
# 1. 构建（要求 Go 1.21+）
.\scripts\build.ps1                         # 编 Windows x64
# .\scripts\build.ps1 -Target linux         # 交叉编 Linux x64

# 2. 注册为 Windows 服务（开机自启、崩溃自动重启）
.\scripts\install-windows.ps1 -Action install
```

其他管理操作：

```powershell
.\scripts\install-windows.ps1 -Action status    # 查看状态
.\scripts\install-windows.ps1 -Action restart   # 重启
.\scripts\install-windows.ps1 -Action stop      # 停止
.\scripts\install-windows.ps1 -Action delete    # 卸载服务
```

### 方式三：Linux 源码编译 + systemd

```bash
# 1. 编译
go build -ldflags="-s -w" -o gmail-proxy ./cmd/proxy

# 2. 部署
sudo cp gmail-proxy /usr/local/bin/
sudo cp configs/config.yaml /etc/gmail-proxy/
sudo useradd -r -s /usr/sbin/nologin gmail-proxy
sudo chown gmail-proxy:gmail-proxy /usr/local/bin/gmail-proxy /etc/gmail-proxy/config.yaml
```

创建 systemd 单元：`/etc/systemd/system/gmail-proxy.service`

```ini
[Unit]
Description=Gmail Proxy (TLS Forwarder)
After=network.target

[Service]
Type=simple
User=gmail-proxy
ExecStart=/usr/local/bin/gmail-proxy -config /etc/gmail-proxy/config.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

启动并设为开机自启：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gmail-proxy
sudo journalctl -u gmail-proxy -f     # 实时日志
```

---

## 客户端配置

### DNS 解析

内网 DNS 服务器添加两条记录（或在客户端 `hosts` 文件里手动加）：

| 主机名 | 指向 |
|--------|------|
| `pop.localdomain.com` | Gmail Proxy 服务器的**内网 IP** |
| `smtp.localdomain.com` | Gmail Proxy 服务器的**内网 IP** |

> 如果你不想改 DNS，也可以直接填服务器内网 IP，只是证书校验需要客户端关闭主机名校验——**不推荐**。

### 邮箱客户端填写

| 字段 | 值 |
|------|-----|
| **收件服务器 (POP3)** | `pop.localdomain.com` |
| 端口 | `995` |
| 加密 | **TLS / SSL** |
| 用户名 | `your@gmail.com` |
| 密码 | 你的 Gmail **应用专用密码**（不是 Google 登录密码） |
| | |
| **发件服务器 (SMTP)** | `smtp.localdomain.com` |
| 端口 | `465` |
| 加密 | **TLS / SSL** |
| 用户名 | `your@gmail.com` |
| 密码 | 同上 |

### 开启 Gmail 应用专用密码

1. 打开 https://myaccount.google.com/security
2. 「两步验证」已开启 → 下面会多出「应用专用密码」入口
3. 生成一个新的应用密码（16 位），填到邮箱客户端里即可

---

## 防火墙与出站代理

### 客户端所在机器

确保能**访问代理服务器的 995 和 465 端口**（TCP），不能被内部防火墙挡住。

### 代理服务器

需要**能访问**以下目标：

| 目标 | 用途 |
|------|------|
| `pop.gmail.com:995/tcp` | POP3S |
| `smtp.gmail.com:465/tcp` | SMTPS |

**如果服务器自己也没有出网直连权限**（场景 B），把上面两个出站连接改成：

| 目标 | 用途 |
|------|------|
| `HTTP代理ip:10808/tcp` | 所有 Gmail 流量都通过它走 CONNECT 隧道 |

`config.yaml` 里填 `outbound_proxy: "代理ip:10808"` 即可。

### 验证代理是否支持 CONNECT

如果 10808 是 HTTP 代理：

```bash
# 这条命令会向代理发送 CONNECT 请求并读 Gmail 995 的 banner
curl -v --proxy http://代理ip:10808 \
  --request-target "" \
  telnet://pop.gmail.com:995 \
  --max-time 10
```

看到 `* Proxy sent CONNECT response: HTTP/1.1 200 Connection established` 就说明能用。

如果你的 10808 其实是 **SOCKS5** 代理，当前版本还不支持，可以提 issue 或者自己加（代码量差不多，就是把 `dialViaHTTPConnect` 换成 SOCKS5 握手）。

---

## 运行日志示例

```
==========================================
  Gmail Proxy - TLS 透传代理
==========================================
  pop3s   :995  ->  (HTTP CONNECT 10.0.0.1:10808)  ->  pop.gmail.com:995
  smtps   :465  ->  (HTTP CONNECT 10.0.0.1:10808)  ->  smtp.gmail.com:465
------------------------------------------
按 Ctrl+C 退出

[pop3s] 新连接  192.168.1.50:53812  <->  pop.gmail.com:995
[pop3s] 连接结束  192.168.1.50:53812
[smtps] 新连接  192.168.1.50:53900  <->  smtp.gmail.com:465
```

---

## 常见问题

**Q: 客户端提示「证书不受信任」？**
A: 因为你填的是 `pop.localdomain.com`，Gmail 证书里的 CN 是 `pop.gmail.com`，正常应该不匹配。但实际上 Gmail 证书同时包含了 CN 和 SAN，现代邮箱客户端通常只做 SAN 校验，只要你在客户端的「证书例外」里手动信任一次，或者让 DNS 用真实的 `pop.gmail.com` 指向代理服务器（前提是服务器不与真实 Gmail 冲突），就能通过。

**Q: 能不能用 Gmail 的 StartTLS（587 / 110）？**
A: 可以，把 `listen` 和 `upstream` 改成对应的端口就行：

```yaml
  - name: smtp-starttls
    listen: ":587"
    upstream: "smtp.gmail.com:587"
  - name: pop3-starttls
    listen: ":110"
    upstream: "pop.gmail.com:110"
```

TLS 会由客户端先发 STARTTLS 再升级，代理全程做透明转发，不影响。

**Q: 多并发连接能扛住吗？**
A: 每个连接只会 spawn 2 个 goroutine + 一个 IO copy，资源非常轻。一台低配服务器扛几十上百并发没问题。

**Q: 有没有日志文件？**
A: 直接 stdout。systemd / docker 都有自己的日志收集（`journalctl` / `docker logs`），Windows 服务模式会写事件日志。如果要文件，自己包一层 supervisor 或用 NSSM 的输出重定向即可。

---

## 技术栈

- Go 1.21+，**只有标准库**（`net` / `net/http` / `io` / `bufio` / `encoding/base64`）
- 无 YAML 库依赖：手写最小 YAML 解析器支持本项目的嵌套结构
- 编译产物单文件，Windows ~5MB，Linux ~7MB（-ldflags="-s -w" 去符号表）
