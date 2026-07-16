# 3xui-lite Linux VPS 安装说明

## 包含内容

- `3xui-lite` — 管理面板（linux/amd64）
- `bin/xray` — Xray-core
- `bin/sing-box` — sing-box
- `bin/geoip.dat` / `geosite.dat` — Xray 路由数据
- `install.sh` — systemd 一键安装
- `start.sh` / `stop.sh` — 前台/后台简易启停

## 系统要求

- Linux x86_64（amd64）
- 常见发行版：Ubuntu 20.04+/Debian 11+/CentOS 8+ 等
- root 权限（systemd 安装）
- 开放面板端口 **18080**，以及你配置的入站端口（如 **8443**）

## 安装（推荐）

```bash
# 上传压缩包后
tar -xzf 3xui-lite-linux-amd64.tar.gz
cd 3xui-lite-linux-amd64
sudo bash install.sh
```

浏览器访问：

```
http://你的服务器公网IP:18080
```

默认账号：**admin / admin**（登录后立刻改密）

## 常用命令

```bash
systemctl status 3xui-lite
systemctl restart 3xui-lite
systemctl stop 3xui-lite
journalctl -u 3xui-lite -f
```

## 防火墙示例

```bash
# Ubuntu / Debian
ufw allow 18080/tcp
ufw allow 8443/tcp
ufw reload

# firewalld (CentOS)
firewall-cmd --permanent --add-port=18080/tcp
firewall-cmd --permanent --add-port=8443/tcp
firewall-cmd --reload
```

云厂商安全组也要放行对应端口。

## 不装 systemd（手动运行）

```bash
cd 3xui-lite-linux-amd64
bash start.sh          # 后台
# bash stop.sh         # 停止
```

或前台：

```bash
export XUI_LISTEN=0.0.0.0:18080
export XUI_DATA=$PWD/data
export XRAY_BIN=$PWD/bin/xray
export SINGBOX_BIN=$PWD/bin/sing-box
./3xui-lite
```

## 使用建议

1. 登录面板 → **设置** 填写公网域名/IP（分享链接用）
2. **入站管理** → 一键生成 **VLESS Reality**
3. **内核** 页确认 Xray 或 sing-box 运行中
4. 点 **二维码** 导入客户端
5. 云安全组放行入站端口

## 卸载

```bash
sudo bash uninstall.sh
# 或
sudo INSTALL_DIR=/opt/3xui-lite bash uninstall.sh
```

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `XUI_LISTEN` | `0.0.0.0:18080` | 面板监听 |
| `XUI_DATA` | `./data` | 数据库与配置 |
| `XRAY_BIN` | `./bin/xray` | Xray 路径 |
| `SINGBOX_BIN` | `./bin/sing-box` | sing-box 路径 |

## 安全提示

- 务必修改默认密码
- 建议面板仅本机 + 反向代理 HTTPS，或限制来源 IP
- 遵守当地法律法规与云厂商条款
