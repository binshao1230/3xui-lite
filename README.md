# 3xui-lite

精简版 Xray / sing-box 管理面板（Go + 内嵌 Web UI）。

## 功能

- **双内核**：Xray / sing-box 可手动切换（同时只运行一个）
- **协议**：VLESS（TCP/WS/gRPC，none/TLS/Reality）、Shadowsocks 2022、中转
- **一键生成**入站（自动端口、密钥、客户端、分享链接、二维码）
- **在线更新**：设置页一键从 GitHub Release 升级面板与内核
- **深色 / 浅色**主题切换
- 无订阅、无 WARP（有意精简）

## 截图入口

浏览器打开面板后：仪表盘 → 一键生成 → 入站管理 / 内核 / 设置。

## 快速开始（Windows 开发）

依赖：Go 1.22+，以及 `bin/xray.exe`、`bin/sing-box.exe`（可从官方 Release 下载）。

```powershell
cd 3xui-lite
go mod tidy
go build -o 3xui-lite.exe ./cmd/server
$env:XUI_LISTEN="127.0.0.1:18080"
$env:XRAY_BIN="$PWD\bin\xray.exe"
$env:SINGBOX_BIN="$PWD\bin\sing-box.exe"
.\3xui-lite.exe
```

访问：http://127.0.0.1:18080  
默认账号：`admin` / `admin`（请立即修改）

## Linux VPS 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/binshao1230/3xui-lite/main/install.sh | sudo bash
```

或：

```bash
wget -qO- https://raw.githubusercontent.com/binshao1230/3xui-lite/main/install.sh | sudo bash
```

- 面板：`http://服务器IP:18080`
- 账号：`admin` / `admin`（请立即修改）
- 安装目录：`/opt/3xui-lite`
- 服务名：`3xui-lite`

可选参数：

```bash
# 自定义监听与目录
curl -fsSL https://raw.githubusercontent.com/binshao1230/3xui-lite/main/install.sh \
  | sudo env XUI_LISTEN=0.0.0.0:18080 INSTALL_DIR=/opt/3xui-lite bash
```

### 离线 / 本地包安装

从 [Releases](https://github.com/binshao1230/3xui-lite/releases) 下载 `3xui-lite-linux-amd64.tar.gz`：

```bash
tar -xzf 3xui-lite-linux-amd64.tar.gz
cd 3xui-lite-linux-amd64
sudo bash install.sh
```

### 自行编译

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o 3xui-lite ./cmd/server
```

详见 `scripts/linux/README-VPS.md`。

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `XUI_LISTEN` | `127.0.0.1:18080` | 面板监听（VPS 建议 `0.0.0.0:18080`） |
| `XUI_DATA` | `./data` | 数据库与配置目录 |
| `XRAY_BIN` | `./bin/xray[.exe]` | Xray 路径 |
| `SINGBOX_BIN` | `./bin/sing-box[.exe]` | sing-box 路径 |

## 目录结构

```
cmd/server/          入口
internal/
  api/               HTTP API
  config/
  core/              双内核切换
  database/
  gen/               一键密钥/端口
  models/
  proc/              进程管理
  singbox/           sing-box 配置生成
  xray/              Xray 配置生成
web/static/          前端
scripts/linux/       VPS 安装脚本
```

## 安全说明

- 修改默认密码
- 生产环境建议 HTTPS 反代或限制来源 IP
- 请遵守当地法律法规与云厂商条款

## License

MIT（若未另行声明）
