---
name: browser-linux-troubleshooting
description: "修复 Claw Acosmi 在 Linux 上控制浏览器时的 Chrome/Brave/Edge/Chromium CDP 启动问题"
---

# 浏览器故障排除（Linux）

## 问题："Failed to start Chrome CDP on port 18800"

Claw Acosmi 的浏览器控制服务无法启动 Chrome/Brave/Edge/Chromium，报错：

```
{"error":"Error: Failed to start Chrome CDP on port 18800 for profile \"openacosmi\"."}
```

### 根本原因

在 Ubuntu（及许多 Linux 发行版）上，默认的 Chromium 安装是一个 **snap 包**。snap 的 AppArmor 沙箱限制干扰了 Claw Acosmi 生成和监控浏览器进程的方式。

`apt install chromium` 安装的是一个重定向到 snap 的桩包 — 并非真正的浏览器。

### 方案 1：安装 Google Chrome（推荐）

安装官方 Google Chrome `.deb` 包：

```bash
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo dpkg -i google-chrome-stable_current_amd64.deb
sudo apt --fix-broken install -y
```

然后更新配置（`~/.openacosmi/openacosmi.json`）：

```json
{
  "browser": {
    "enabled": true,
    "executablePath": "/usr/bin/google-chrome-stable",
    "headless": true,
    "noSandbox": true
  }
}
```

### 方案 2：snap Chromium + Attach-Only 模式

如果必须使用 snap Chromium，配置 attach-only 模式：

```json
{
  "browser": {
    "enabled": true,
    "attachOnly": true,
    "headless": true,
    "noSandbox": true
  }
}
```

手动启动 Chromium：

```bash
chromium-browser --headless --no-sandbox --disable-gpu \
  --remote-debugging-port=18800 \
  --user-data-dir=$HOME/.openacosmi/browser/openacosmi/user-data \
  about:blank &
```

可选：创建 systemd 用户服务以自动启动（`~/.config/systemd/user/openacosmi-browser.service`）。

### 验证

```bash
curl -s http://127.0.0.1:18791/ | jq '{running, pid, chosenBrowser}'
curl -s -X POST http://127.0.0.1:18791/start
curl -s http://127.0.0.1:18791/tabs
```

### 配置参考

| 选项 | 描述 | 默认值 |
|------|------|--------|
| `browser.enabled` | 启用浏览器控制 | `true` |
| `browser.executablePath` | Chromium 系浏览器二进制路径 | 自动检测 |
| `browser.headless` | 无 GUI 运行 | `false` |
| `browser.noSandbox` | 添加 `--no-sandbox` 标志 | `false` |
| `browser.attachOnly` | 不启动浏览器，仅附加 | `false` |
| `browser.cdpPort` | Chrome DevTools Protocol 端口 | `18800` |

### 问题："Chrome extension relay is running, but no tab is connected"

使用 `chrome` 配置文件（扩展中继）但没有标签页附加。

修复：设置 `browser.defaultProfile: "openacosmi"` 使用托管浏览器，或安装扩展并点击图标附加标签页。
