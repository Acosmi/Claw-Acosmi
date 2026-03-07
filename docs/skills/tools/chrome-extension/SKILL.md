---
name: chrome-extension
description: "Chrome MV3 扩展：让 Claw Acosmi 通过 chrome.debugger API 驱动您现有的 Chrome 标签页"
---

# Chrome 扩展（浏览器中继）

Claw Acosmi Chrome 扩展可以让 Agent 控制您**现有的 Chrome 标签页**（您日常使用的 Chrome 窗口），而无需启动单独的 openacosmi 托管 Chrome 配置文件。

通过 **Chrome 工具栏按钮**进行附加/分离。

## 架构

```
┌────────────────────┐   WebSocket    ┌──────────────────┐
│  Chrome Extension  │ ◄────────────▶ │  ExtensionRelay   │
│  (browser-         │               │  (Go backend)     │
│   extension/)      │               │  端口: gateway    │
│                    │               │    port + 3       │
│  background.js     │               └────────┬──────────┘
│  (Service Worker)  │                        │
│  ├─ chrome.debugger│                        │
│  ├─ tab 管理       │                        ▼
│  └─ relay 重连     │             ┌──────────────────┐
│                    │             │ BrowserController │
│  popup.html/js     │             │ (Agent 工具层)    │
│  (工具栏 UI)       │             └──────────────────┘
└────────────────────┘
```

## 组件说明

| 组件 | 说明 |
|------|------|
| **Chrome MV3 扩展** | 使用 `chrome.debugger` API 附加到标签页，将 CDP 命令转发到 Relay 服务器 |
| **ExtensionRelay** | Go 后端 WebSocket 服务器，桥接扩展和 Agent 工具层。自动生成认证 token |
| **BrowserController** | Agent 工具统一接口，通过 `SendCDPToExtension()` 向扩展发送 CDP 命令 |

## 安装

详细安装步骤参见 `docs/使用帮助/浏览器自动化扩展安装使用说明.md`。

快速步骤：
1. Chrome → `chrome://extensions` → 启用"开发者模式"
2. "加载已解压的扩展" → 选择仓库根目录下 `browser-extension/` 目录
3. 固定扩展到工具栏

## 端口

Relay 服务器端口通过 Gateway 端口自动推导：

```
relay_port = gateway_port + 3
```

| 环境 | Gateway 端口 | Relay 端口 | 扩展默认连接 |
|------|-------------|-----------|-------------|
| 开发 | 19001 | 19004 | `ws://127.0.0.1:19004/ws` |
| 生产 | 18789 | 18792 | `ws://127.0.0.1:18792/ws` |

扩展弹窗中可手动修改 Relay URL。Token 支持自动发现（通过 `/json/version` 端点）。

## 使用

系统内置 `chrome` profile 指向扩展中继端口。Agent 工具中使用 `browser` 工具 + `profile="chrome"` 参数。

自定义 profile 配置参见 `docs/cli/browser.md`。

## 附加/分离

- 打开您想让 Agent 控制的标签页。
- 点击扩展图标 → 在 tab 列表中点击 **Attach** 按钮。
- 附加后工具栏图标显示绿色 `ON` 标记。
- 再次点击 **Detach** 按钮或使用底部 **Detach All** 断开所有标签页。
- 也可使用 **Attach Current Tab** 快速附加当前活跃标签页。

## 控制范围

- **不会**自动控制"您正在查看的标签页"。
- 仅控制**您通过点击扩展 UI 显式附加的标签页**。
- 附加后，Agent 通过 `chrome.debugger` API 获得该标签页的完整 CDP 控制权。
- 自动启用的 CDP domains: `Page`, `Runtime`, `DOM`, `Accessibility`。

## 状态标记

| 标记 | 含义 |
|------|------|
| `ON` (绿色) | 已连接 Relay 且有标签页已附加，Agent 可驱动 |
| `...` (橙色) | 正在连接 Relay 服务器 |
| `!` (红色) | Relay 不可达（常见：Gateway 未运行或端口不对） |
| 无标记 | 已连接 Relay 但无标签页附加 |

## 自动重连

扩展内置自动重连机制：
- 最多 10 次尝试
- 延迟递增（3s × min(attempt, 5)）
- Relay 恢复后自动重连并发送 tab 列表

## 认证

- Relay 启动时生成 256-bit 随机 token
- 扩展通过 `/json/version` 端点自动发现 token（loopback only）
- 也可在弹窗 UI 中手动输入 token
- 所有 WebSocket 连接需携带 `?token=xxx`

## 沙箱

如果 Agent session 是沙箱模式，browser 工具默认目标为沙箱。允许 host 控制：

```json5
{
  agents: {
    defaults: {
      sandbox: {
        browser: {
          allowHostControl: true,
        },
      },
    },
  },
}
```

## 安全影响

将此功能视为给 Agent "在您浏览器上操作的双手"。

- 扩展使用 Chrome 的 `chrome.debugger` API。附加后，Agent 可以：
  - 在该标签页中点击/输入/导航
  - 读取页面内容
  - 访问该标签页已登录会话能访问的任何内容
- **这不像**专用的 openacosmi 托管配置文件那样隔离。

建议：
- 推荐为扩展中继使用专用的 Chrome 配置文件。
- 保持 Gateway 和所有 node host 仅在 tailnet 中。
- 避免在局域网或公共互联网上暴露 Relay 端口。
- Relay 仅监听 `127.0.0.1`，不接受外部连接。

## CLI 参考

完整 CLI 命令列表参见 `docs/cli/browser.md`（Rust CLI）。
